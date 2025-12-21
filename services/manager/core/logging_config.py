import logging.config
import yaml
import os

def setup_logging():
    """
    YAML設定ファイルを読み込み、ロギングを初期化します。
    """
    config_path = os.getenv("LOG_CONFIG_PATH", "/app/config/manager_log.yaml")
    if os.path.exists(config_path):
        with open(config_path, "r") as f:
            config = yaml.safe_load(f)
            logging.config.dictConfig(config)
    else:
        # フォールバック
        logging.basicConfig(level=logging.INFO)
