from . import down, up
from tools.cli.core import logging


def run(args):
    """
    Fully reset the environment: docker compose down -v -> esb up --build
    """
    logging.warning("This command will PERMANENTLY DELETE all database tables and S3 buckets.")

    # Skip confirmation if --yes is provided
    if getattr(args, "yes", False):
        logging.info("Skipping confirmation (--yes).")
    else:
        try:
            confirm = input(
                f"{logging.Color.YELLOW}Are you sure you want to proceed? [y/N]: {logging.Color.END}"
            )
        except (EOFError, KeyboardInterrupt):
            print()  # Newline.
            logging.info("Reset cancelled.")
            return

        if confirm.lower() not in ["y", "yes"]:
            logging.info("Reset cancelled.")
            return

    logging.step("Resetting environment...")

    # Simple class for argument pass-through.
    class ResetArgs:
        def __init__(self, **kwargs):
            for k, v in kwargs.items():
                setattr(self, k, v)

    rmi = getattr(args, "rmi", False)
    if rmi:
        logging.info("Deleting all containers, volumes, and images...")
    else:
        logging.info("Deleting all containers and volumes...")
    
    down_args = ResetArgs(volumes=True, rmi=rmi)
    down.run(down_args)

    # 2. Restart (forced build).
    logging.info("Rebuilding and starting services...")
    up_args = ResetArgs(build=True, detach=True)
    up.run(up_args)

    logging.success("Environment has been successfully reset.")
