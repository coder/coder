FROM codercom/enterprise-base:ubuntu

# Install dependencies needed for Claude Code and VS Code
# Using NodeSource to get a more recent version of Node.js
RUN apt-get update && \
    apt-get install -y \
    curl \
    git \
    jq \
    screen \
    sudo \
    wget \
    && curl -fsSL https://deb.nodesource.com/setup_18.x | bash - \
    && apt-get install -y nodejs \
    && echo "Node.js version: $(node -v)" \
    && echo "npm version: $(npm -v)" \
    && rm -rf /var/lib/apt/lists/* 

# Create global npm directories with proper permissions
RUN mkdir -p /usr/local/lib/node_modules && \
    chmod 777 /usr/local/lib/node_modules

# Claude Code CLI is installed via npm
# Install the npm package directly for immediate availability
RUN npm install -g @anthropic-ai/claude-code

# Set up user for Coder
ARG USER=coder
RUN useradd --groups sudo --create-home --shell /bin/bash ${USER} \
    && echo "${USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/${USER} \
    && chmod 0440 /etc/sudoers.d/${USER}

USER ${USER}
WORKDIR /home/${USER}

# Configure Claude for dashboard reporting
ENV CODER_MCP_APP_STATUS_SLUG=claude