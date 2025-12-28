# Root DinD Container
# Parent container: run the Docker daemon and manage inner containers.

FROM docker:24-dind

# Install required tools.
RUN apk add --no-cache bash curl git python3 py3-pip

WORKDIR /app

# Copy the entire project.
COPY . /app/

# Install into the system environment without a virtualenv (DinD container only).
# Use --break-system-packages to bypass PEP 668 constraints.
RUN pip install --break-system-packages -e ".[dev]"

# Entry point script.
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
