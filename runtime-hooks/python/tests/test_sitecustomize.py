import json
import os
import subprocess
import sys
from pathlib import Path

RESULT_PREFIX = "RESULT_JSON="
REPO_ROOT = Path(__file__).resolve().parents[3]
SITECUSTOMIZE_SITE_PACKAGES = (
    REPO_ROOT / "runtime-hooks" / "python" / "sitecustomize" / "site-packages"
)


def _run_sitecustomize_script(
    script: str, extra_env: dict[str, str] | None = None
) -> dict[str, object]:
    # sitecustomize mutates global process state (e.g. boto3/client monkey patches and logging),
    # so each assertion runs in a subprocess to avoid cross-test contamination.
    env = os.environ.copy()
    pythonpath = str(SITECUSTOMIZE_SITE_PACKAGES)
    if env.get("PYTHONPATH"):
        pythonpath = f"{pythonpath}{os.pathsep}{env['PYTHONPATH']}"
    env["PYTHONPATH"] = pythonpath

    env["AWS_ACCESS_KEY_ID"] = "dummy"
    env["AWS_SECRET_ACCESS_KEY"] = "dummy"
    env["AWS_DEFAULT_REGION"] = "us-east-1"
    env["AWS_EC2_METADATA_DISABLED"] = "true"
    env["S3_ENDPOINT"] = "http://127.0.0.1:4566"
    if extra_env:
        env.update(extra_env)

    completed = subprocess.run(
        [sys.executable, "-c", script],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )

    if completed.returncode != 0:
        raise AssertionError(
            "subprocess failed\n"
            f"exit={completed.returncode}\n"
            f"stdout:\n{completed.stdout}\n"
            f"stderr:\n{completed.stderr}"
        )

    for line in reversed(completed.stdout.splitlines()):
        if line.startswith(RESULT_PREFIX):
            return json.loads(line[len(RESULT_PREFIX) :])

    raise AssertionError(
        "result line not found in subprocess output\n"
        f"stdout:\n{completed.stdout}\n"
        f"stderr:\n{completed.stderr}"
    )


def test_sitecustomize_patches_boto3_client_and_session_client():
    result = _run_sitecustomize_script(
        """
import json
import boto3
from boto3.session import Session
import sitecustomize  # noqa: F401

session = Session(
    aws_access_key_id="dummy",
    aws_secret_access_key="dummy",
    region_name="us-east-1",
)

data = {
    "boto3_patch_flag": getattr(boto3.client, "_sitecustomize_boto3_patch", False),
    "session_patch_flag": getattr(Session.client, "_sitecustomize_boto3_session_patch", False),
    "boto3_s3_endpoint": boto3.client("s3").meta.endpoint_url,
    "session_s3_endpoint": session.client("s3").meta.endpoint_url,
    "boto3_logs_describe": boto3.client("logs")._make_api_call("DescribeLogGroups", {}),
    "session_logs_describe": session.client("logs")._make_api_call("DescribeLogGroups", {}),
}
print("RESULT_JSON=" + json.dumps(data, sort_keys=True))
"""
    )

    assert result["boto3_patch_flag"] is True
    assert result["session_patch_flag"] is True
    assert result["boto3_s3_endpoint"] == "http://127.0.0.1:4566"
    assert result["session_s3_endpoint"] == "http://127.0.0.1:4566"
    assert result["boto3_logs_describe"] == {"logGroups": []}
    assert result["session_logs_describe"] == {"logGroups": []}


def test_sitecustomize_reload_keeps_session_patch_active():
    result = _run_sitecustomize_script(
        """
import importlib
import json
import boto3
from boto3.session import Session
import sitecustomize

importlib.reload(sitecustomize)

session = Session(
    aws_access_key_id="dummy",
    aws_secret_access_key="dummy",
    region_name="us-east-1",
)

data = {
    "boto3_patch_flag": getattr(boto3.client, "_sitecustomize_boto3_patch", False),
    "session_patch_flag": getattr(Session.client, "_sitecustomize_boto3_session_patch", False),
    "session_s3_endpoint": session.client("s3").meta.endpoint_url,
    "session_logs_describe": session.client("logs")._make_api_call("DescribeLogGroups", {}),
}
print("RESULT_JSON=" + json.dumps(data, sort_keys=True))
"""
    )

    assert result["boto3_patch_flag"] is True
    assert result["session_patch_flag"] is True
    assert result["session_s3_endpoint"] == "http://127.0.0.1:4566"
    assert result["session_logs_describe"] == {"logGroups": []}


def test_sitecustomize_presign_uses_public_endpoint_only():
    result = _run_sitecustomize_script(
        """
import json
import boto3
from urllib.parse import urlparse
import sitecustomize  # noqa: F401

client = boto3.client("s3")
presigned_url = client.generate_presigned_url(
    "get_object",
    Params={"Bucket": "bucket-name", "Key": "sample.txt"},
    ExpiresIn=600,
)
presigned_post = client.generate_presigned_post(
    "bucket-name",
    "sample.txt",
    ExpiresIn=600,
)

data = {
    "client_endpoint": client.meta.endpoint_url,
    "presigned_url_netloc": urlparse(presigned_url).netloc,
    "presigned_url_scheme": urlparse(presigned_url).scheme,
    "presigned_post_netloc": urlparse(presigned_post["url"]).netloc,
    "presigned_post_scheme": urlparse(presigned_post["url"]).scheme,
}
print("RESULT_JSON=" + json.dumps(data, sort_keys=True))
""",
        extra_env={"S3_PRESIGN_ENDPOINT": "https://public.example.com:8443"},
    )

    assert result["client_endpoint"] == "http://127.0.0.1:4566"
    assert result["presigned_url_netloc"] == "public.example.com:8443"
    assert result["presigned_url_scheme"] == "https"
    assert result["presigned_post_netloc"] == "public.example.com:8443"
    assert result["presigned_post_scheme"] == "https"
