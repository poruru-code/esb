
import os
import sys
import yaml
from pathlib import Path
import questionary
from tools.cli import config as cli_config
from tools.cli.core.cert import ensure_certs
from tools.cli.core.trust_store import install_root_ca

def run(args):
    """
    Run the interactive wizard and generate generator.yml.
    """
    print("🚀 Initializing Edge Serverless Box configuration...")

    # 1. Find the template file.
    # Priority: 1) main parser --template (cli_config.TEMPLATE_YAML)
    #           2) subparser --template (args.template)
    #           3) current directory search
    template_path = None
    
    # Use cli_config.TEMPLATE_YAML if set (via main parser).
    if cli_config.TEMPLATE_YAML and cli_config.TEMPLATE_YAML.exists():
        template_path = cli_config.TEMPLATE_YAML.resolve()
    elif args.template:
        template_path = Path(args.template).resolve()
    else:
        # Default search order.
        candidates = [
            Path("template.yaml"),
            Path("template.yml"),
        ]
        for c in candidates:
            if c and c.exists():
                template_path = c.resolve()
                break
    
    if not template_path or not template_path.exists():
        # Prompt for input if not found.
        path_input = questionary.path("Path to SAM template.yaml:").ask()
        if not path_input:
            print("❌ No template provided. Aborting.")
            sys.exit(1)
        template_path = Path(path_input).resolve()

    print(f"ℹ Using template: {template_path}")
    sys.stdout.flush()

    # 2. Load the template and extract parameters.
    from tools.generator.parser import CfnLoader
    try:
        with open(template_path, 'r', encoding='utf-8') as f:
            template_data = yaml.load(f, Loader=CfnLoader)
    except Exception as e:
        print(f"❌ Failed to load template: {e}")
        sys.exit(1)

    parameters = template_data.get("Parameters", {})
    param_values = {}

    if parameters:
        print("\n📝 Configure Parameters:")
        sys.stdout.flush()
        for key, value in parameters.items():
            default_val = value.get("Default", "")
            description = value.get("Description", "")
            prompt_text = f"Value for '{key}'"
            if description:
                prompt_text += f" ({description})"
            
            user_val = questionary.text(prompt_text, default=str(default_val)).ask()
            if user_val is None:
                print("❌ Input cancelled. Aborting.")
                sys.exit(1)
            param_values[key] = user_val

    # 3. Additional settings.
    print("\n⚙ Additional Configuration:")
    sys.stdout.flush()
    
    # Image Tag
    image_tag = questionary.text("Docker Image Tag:", default="latest").ask()
    if image_tag is None:
        print("❌ Input cancelled. Aborting.")
        sys.exit(1)
    
    # Output Directory
    # Default is .esb under the template directory.
    default_output_dir = template_path.parent / ".esb"
    output_dir_input = questionary.path("Output Directory for artifacts:", default=str(default_output_dir)).ask()
    if output_dir_input is None:
        print("❌ Input cancelled. Aborting.")
        sys.exit(1)
    output_dir = Path(output_dir_input).resolve()

    # 4. Generate generator.yml.
    # Convert paths to be relative to the template for portability.
    base_dir = template_path.parent
    
    def to_rel(p: Path) -> str:
        try:
            return os.path.relpath(p, base_dir)
        except ValueError:
            return str(p)

    generator_config = {
        "app": {
            "name": "", # Keep empty for now; could add a prefix later.
            "tag": image_tag
        },
        "paths": {
            "sam_template": to_rel(template_path),
            "output_dir": to_rel(output_dir) + "/"
        }
    }
    
    if param_values:
        generator_config["parameters"] = param_values

    # Save location: create generator.yml in the same directory as the template.
    save_path = template_path.parent / "generator.yml"
    
    # Check for existing file.
    if save_path.exists():
        overwrite = questionary.confirm(f"File {save_path} already exists. Overwrite?").ask()
        if not overwrite:
            print("Aborted.")
            sys.exit(0)

    try:
        with open(save_path, 'w', encoding='utf-8') as f:
            yaml.dump(generator_config, f, default_flow_style=False, sort_keys=False, allow_unicode=True)
        print(f"\n✅ Configuration saved to: {save_path}")
        print("You can now run 'esb build' to generate Dockerfiles.")
    except Exception as e:
        print(f"❌ Failed to save config: {e}")
        sys.exit(1)

    # 5. Prepare SSL certificates and install OS trust.
    print("\n🔐 Preparing SSL certificates and trust store...")
    try:
        from tools.cli.config import DEFAULT_CERT_DIR
        ensure_certs(DEFAULT_CERT_DIR)
        install_root_ca(DEFAULT_CERT_DIR / "rootCA.crt")
        print("✅ Root CA installed successfully.")
    except Exception as e:
        print(f"⚠️ Failed to prepare certificates/trust store: {e}")
        print("You may need to run this command with administrative privileges or manually install the CA cert.")
