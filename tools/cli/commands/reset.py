from tools.cli.core import context, logging

from . import build, down, up


def run(args):
    """
    Fully reset the environment: docker compose down -v -> esb up --build
    """
    context.enforce_env_arg(args, require_built=True)

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

    env = getattr(args, "env", "default")

    rmi = getattr(args, "rmi", False)
    if rmi:
        logging.info("Deleting all containers, volumes, and images...")
    else:
        logging.info("Deleting all containers and volumes...")
    
    down_args = ResetArgs(volumes=True, rmi=rmi, env=env)
    down.run(down_args)

    # 2. Restart (forced build).
    logging.info("Rebuilding and starting services...")
    
    # Explicitly run function build (since 'up' doesn't do it anymore)
    # We pass the same args, assuming 'build' flag is acceptable or we pass build-specific args if needed.
    # build.run() expects args to have build-related attributes.
    # Creating a BuildArgs shim or reusing args?
    # args in 'reset' might not have 'no_cache' etc unless we add them to reset parser.
    # For now, we perform a standard build.
    class BuildArgs(ResetArgs):
        pass
    
    # Default to standard build
    build_args = BuildArgs(no_cache=False, dry_run=False, verbose=getattr(args, "verbose", False), file=getattr(args, "file", []), env=env)
    build.run(build_args)

    up_args = ResetArgs(build=True, detach=True, env=env)
    up.run(up_args)

    logging.success("Environment has been successfully reset.")
