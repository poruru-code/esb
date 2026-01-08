from unittest.mock import patch, MagicMock
from pathlib import Path
from tools.cli.commands.reset import run as run_reset

# Common patches
@patch("builtins.input")
@patch("tools.cli.commands.reset.build.run")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
@patch("tools.cli.commands.reset.context.enforce_env_arg")
def test_reset_command_cancel(mock_enforce, mock_up, mock_down, mock_build, mock_input):
    """Ensure reset is canceled when entering 'n'."""
    mock_input.return_value = "n"
    args = MagicMock()
    args.yes = False  # Explicitly set --yes to False.
    # explicit attributes to satisfy type checks
    args.env = "default"
    args.rmi = False
    args.build = False
    args.file = []
    
    with patch("tools.cli.config.TEMPLATE_YAML", Path("dummy.yaml")):
        run_reset(args)

    # Neither down nor up should be called.
    mock_down.assert_not_called()
    mock_up.assert_not_called()
    mock_build.assert_not_called()


@patch("builtins.input")
@patch("tools.cli.commands.reset.build.run")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
@patch("tools.cli.commands.reset.context.enforce_env_arg")
def test_reset_command_proceed(mock_enforce, mock_up, mock_down, mock_build, mock_input):
    """Ensure down -v and up --build are called when entering 'y'."""
    mock_input.return_value = "y"
    args = MagicMock()
    # Explicitly set string/bool attributes to avoid MagicMock leaking into path functions
    args.yes = False
    args.rmi = False
    args.env = "default"
    args.build = False
    args.file = []
    args.verbose = False

    with patch("tools.cli.config.TEMPLATE_YAML", Path("dummy.yaml")):
        run_reset(args)

    # 1. Ensure down.run(volumes=True) was called.
    mock_down.assert_called_once()
    called_down_args = mock_down.call_args[0][0]
    assert called_down_args.volumes is True
    assert called_down_args.rmi is False
    
    # Ensure build.run was called
    mock_build.assert_called_once()

    # 2. Ensure up.run(build=True, detach=True) was called.
    mock_up.assert_called_once()
    called_up_args = mock_up.call_args[0][0]
    assert called_up_args.build is True


@patch("builtins.input")
@patch("tools.cli.commands.reset.build.run")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
@patch("tools.cli.commands.reset.context.enforce_env_arg")
def test_reset_command_with_rmi(mock_enforce, mock_up, mock_down, mock_build, mock_input):
    """Ensure reset --rmi passes rmi=True to down."""
    mock_input.return_value = "y"
    args = MagicMock()
    args.yes = False
    args.rmi = True
    args.env = "default"
    args.build = False
    args.file = []
    args.verbose = False

    with patch("tools.cli.config.TEMPLATE_YAML", Path("dummy.yaml")):
        run_reset(args)

    # Ensure down.run(volumes=True, rmi=True) was called.
    mock_down.assert_called_once()
    called_down_args = mock_down.call_args[0][0]
    assert called_down_args.volumes is True
    assert called_down_args.rmi is True
    
    mock_build.assert_called_once()
