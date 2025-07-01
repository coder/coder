ALTER TABLE template_version_terraform_values ADD COLUMN cached_module_files uuid references files(id);
