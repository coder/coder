ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'dashboard';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'cli';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'ssh_connection';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'vscode_connection';
ALTER TYPE build_reason ADD VALUE IF NOT EXISTS 'jetbrains_connection';
