BEGIN;
CREATE TYPE display_app AS ENUM ('vscode', 'vscode_insiders', 'web_terminal', 'ssh_helper', 'port_forwarding_helper');
ALTER TABLE workspace_agents ADD column display_apps display_app[] DEFAULT '{vscode, vscode_insiders, web_terminal, ssh_helper, port_forwarding_helper}';
COMMIT;
