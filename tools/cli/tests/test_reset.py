from unittest.mock import patch, MagicMock
from tools.cli.commands.reset import run as run_reset


@patch("builtins.input")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
def test_reset_command_cancel(mock_up, mock_down, mock_input):
    """Ensure reset is canceled when entering 'n'."""
    mock_input.return_value = "n"
    args = MagicMock()
    args.yes = False  # Explicitly set --yes to False.

    run_reset(args)

    # Neither down nor up should be called.
    mock_down.assert_not_called()
    mock_up.assert_not_called()


@patch("builtins.input")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
def test_reset_command_proceed(mock_up, mock_down, mock_input):
    """Ensure down -v and up --build are called when entering 'y'."""
    mock_input.return_value = "y"
    args = MagicMock()
    args.yes = False
    args.rmi = False

    run_reset(args)

    # 1. Ensure down.run(volumes=True) was called.
    mock_down.assert_called_once()
    called_down_args = mock_down.call_args[0][0]
    assert called_down_args.volumes is True
    assert called_down_args.rmi is False

    # 2. Ensure up.run(build=True, detach=True) was called.
    mock_up.assert_called_once()
    called_up_args = mock_up.call_args[0][0]
    assert called_up_args.build is True


@patch("builtins.input")
@patch("tools.cli.commands.reset.down.run")
@patch("tools.cli.commands.reset.up.run")
def test_reset_command_with_rmi(mock_up, mock_down, mock_input):
    """Ensure reset --rmi passes rmi=True to down."""
    mock_input.return_value = "y"
    args = MagicMock()
    args.yes = False
    args.rmi = True

    run_reset(args)

    # Ensure down.run(volumes=True, rmi=True) was called.
    mock_down.assert_called_once()
    called_down_args = mock_down.call_args[0][0]
    assert called_down_args.volumes is True
    assert called_down_args.rmi is True
