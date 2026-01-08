# Where: tools/cli/commands/watch.py
# What: Watch for file changes and reload services.
# Why: Provide a lightweight hot-reload workflow for CLI users.
import subprocess
import time
from pathlib import Path

import docker
from watchdog.events import FileSystemEventHandler
from watchdog.observers import Observer

from tools.cli import compose as cli_compose
from tools.cli.config import PROJECT_ROOT
from tools.cli.core import logging, proxy
from tools.provisioner import main as provisioner


class SmartReloader(FileSystemEventHandler):
    def __init__(self):
        self.docker_client = docker.from_env()
        self.last_trigger = 0
        self.cooldown = 1.0  # Cooldown to prevent duplicate runs (seconds).

    def on_modified(self, event):
        if event.is_directory:
            return

        current_time = time.time()
        if current_time - self.last_trigger < self.cooldown:
            return

        path = Path(event.src_path)
        filename = path.name

        try:
            # Case 1: template.yaml changed.
            if filename == "template.yaml":
                self.handle_template_change()
                self.last_trigger = time.time()

            # Case 2: Lambda function code changed.
            elif path.suffix == ".py" and "functions" in str(path):
                self.handle_function_change(path)
                self.last_trigger = time.time()

        except Exception as e:
            logging.warning(f"Error during reload: {e}")

    def handle_template_change(self):
        logging.step("Template change detected.")

        # 1. Regenerate config.
        logging.info("Regenerating configs...")
        from tools.cli import config as cli_config
        from tools.cli.commands.build import generator
        from tools.cli.config import E2E_DIR, PROJECT_ROOT, TEMPLATE_YAML

        config_path = E2E_DIR / "generator.yml"
        if not config_path.exists():
            config_path = PROJECT_ROOT / "tests/fixtures/generator.yml"

        config = generator.load_config(config_path)
        # Resolve template path.
        if "paths" not in config:
            config["paths"] = {}
        config["paths"]["sam_template"] = str(TEMPLATE_YAML)

        # Set output directory based on environment name
        env_name = cli_config.get_env_name()
        config["paths"]["output_dir"] = f".esb/{env_name}/"

        generator.generate_files(
            config=config, project_root=PROJECT_ROOT, tag=cli_config.get_image_tag(env_name)
        )

        # 2. Restart Gateway (to apply routing changes).
        logging.info("Restarting Gateway...")
        try:
            subprocess.run(
                cli_compose.build_compose_command(["restart", "gateway"], target="control"),
                check=True,
                capture_output=True,
                env=proxy.prepare_env(),
            )
        except subprocess.CalledProcessError as e:
            logging.error(f"Failed to restart gateway: {e}")

        # 3. Re-provision resources (e.g., add DB tables).
        logging.info("Provisioning resources...")
        provisioner.main(template_path=TEMPLATE_YAML)
        logging.success("System updated.")

    def handle_function_change(self, path: Path):
        try:
            parts = path.parts
            if "functions" in parts:
                idx = parts.index("functions")
                if len(parts) > idx + 1:
                    from tools.cli import config as cli_config

                    func_dir_name = parts[idx + 1]
                    env_tag = cli_config.get_image_tag()
                    image_tag = f"lambda-{func_dir_name}:{env_tag}"

                    from tools.cli.config import E2E_DIR

                    func_dir = E2E_DIR / "functions" / func_dir_name
                    dockerfile_path = func_dir / "Dockerfile"

                    if not dockerfile_path.exists():
                        logging.warning(f"Dockerfile not found at {dockerfile_path}")
                        return

                    logging.step(f"Code change detected: {logging.highlight(func_dir_name)}")

                    # 1. Rebuild the image.
                    print(
                        f"  • Rebuilding image: {logging.highlight(image_tag)} ...",
                        end="",
                        flush=True,
                    )
                    relative_dockerfile = dockerfile_path.relative_to(PROJECT_ROOT).as_posix()
                    self.docker_client.images.build(
                        path=str(PROJECT_ROOT),
                        dockerfile=relative_dockerfile,
                        tag=image_tag,
                        rm=True,
                        buildargs=proxy.docker_build_args(),
                    )
                    print(f" {logging.Color.GREEN}✅{logging.Color.END}")

                    # 2. Stop running old containers.
                    containers = self.docker_client.containers.list(
                        filters={"ancestor": f"{image_tag}"}
                    )
                    if containers:
                        for c in containers:
                            logging.info(f"Killing running container: {logging.highlight(c.name)}")
                            c.kill()

                    logging.success("Function updated.")
        except Exception as e:
            logging.error(f"Failed to update function: {e}")


def run(args):


    proxy.apply_proxy_env()

    logging.step("Watching for changes...")
    print(f"   • {logging.highlight('template.yaml')}: Reconfigures Gateway & Resources")
    print(f"   • {logging.highlight('functions/**/*.py')}: Rebuilds Lambda Images")

    event_handler = SmartReloader()
    observer = Observer()

    observer.schedule(event_handler, str(PROJECT_ROOT), recursive=True)
    observer.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        logging.info("Stopping watcher...")
        observer.stop()
    observer.join()
