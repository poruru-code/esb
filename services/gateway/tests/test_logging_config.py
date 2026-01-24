"""
Where: services/gateway/tests/test_logging_config.py
What: Unit tests for gateway logging configuration.
Why: Validate VictoriaLogs URL overrides for gateway logs.
"""

from services.gateway.core import logging_config


def test_setup_logging_prefers_gateway_victorialogs_url(monkeypatch):
    captured = {}

    def fake_configure_queue_logging(service_name: str, vl_url: str):
        captured["service_name"] = service_name
        captured["vl_url"] = vl_url

    monkeypatch.setenv("GATEWAY_VICTORIALOGS_URL", "http://victorialogs:9428")
    monkeypatch.setenv("VICTORIALOGS_URL", "http://10.88.0.1:9428")
    monkeypatch.setenv("LOG_CONFIG_PATH", "/tmp/esb-missing-logging.yml")
    monkeypatch.delenv("DISABLE_VICTORIALOGS", raising=False)
    monkeypatch.setattr(logging_config, "configure_queue_logging", fake_configure_queue_logging)

    logging_config.setup_logging()

    assert captured["service_name"] == "esb-gateway"
    assert captured["vl_url"] == "http://victorialogs:9428/insert/jsonline"
