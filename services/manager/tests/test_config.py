from services.manager.config import ManagerConfig
from services.common.core.config import BaseAppConfig


def test_manager_config_inheritance():
    """
    TDD: ManagerConfig should inherit from BaseAppConfig.
    """
    assert issubclass(ManagerConfig, BaseAppConfig)


def test_manager_config_fields():
    """
    TDD: ManagerConfig should have IDLE_TIMEOUT_MINUTES and CONTAINERS_NETWORK.
    """
    config = ManagerConfig()
    assert hasattr(config, "IDLE_TIMEOUT_MINUTES")
    assert hasattr(config, "CONTAINERS_NETWORK")
    # Verify defaults
    assert config.IDLE_TIMEOUT_MINUTES == 5
    # CONTAINERS_NETWORK default is "lambda-net" unless overridden by env;
    # In test environment other modules may have set different values
    assert isinstance(config.CONTAINERS_NETWORK, str)


def test_manager_config_lambda_defaults():
    """
    TDD Red Phase: Lambda関連の共通設定が読み込まれることを検証

    BaseAppConfigから継承されるべきフィールド:
    - LAMBDA_PORT: Lambda RIEコンテナのポート番号
    - READINESS_TIMEOUT: コンテナReadinessチェックのタイムアウト
    - DOCKER_DAEMON_TIMEOUT: Docker Daemon起動待機のタイムアウト
    """
    config = ManagerConfig()

    # Inherited from BaseAppConfig
    assert hasattr(config, "LAMBDA_PORT"), "Should have LAMBDA_PORT from BaseAppConfig"
    assert hasattr(config, "READINESS_TIMEOUT"), "Should have READINESS_TIMEOUT from BaseAppConfig"
    assert hasattr(config, "DOCKER_DAEMON_TIMEOUT"), (
        "Should have DOCKER_DAEMON_TIMEOUT from BaseAppConfig"
    )

    # Verify default values
    assert config.LAMBDA_PORT == 8080
    assert config.READINESS_TIMEOUT == 30
    assert config.DOCKER_DAEMON_TIMEOUT == 30
