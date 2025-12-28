import os

from services.common.core.logging_config import configure_queue_logging
from services.common.core.logging_config import setup_logging as common_setup_logging


def setup_logging():
    """
    Load the YAML config and initialize logging.
    Also configure async log delivery to VictoriaLogs.
    """
    config_path = os.getenv("LOG_CONFIG_PATH", "/app/config/gateway_log.yaml")
    common_setup_logging(config_path)

    # Configure async delivery to VictoriaLogs.
    vl_url = os.getenv("VICTORIALOGS_URL", "")
    if vl_url:
        if not vl_url.endswith("/insert/jsonline"):
            vl_url = f"{vl_url.rstrip('/')}/insert/jsonline"
    else:
        vl_host = os.getenv("VICTORIALOGS_HOST", "victorialogs")
        vl_port = os.getenv("VICTORIALOGS_PORT", "9428")
        vl_url = f"http://{vl_host}:{vl_port}/insert/jsonline"
    configure_queue_logging(service_name="esb-gateway", vl_url=vl_url)
