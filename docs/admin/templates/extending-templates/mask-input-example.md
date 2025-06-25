# Mask Input Feature

The `mask_input` styling option allows you to hide sensitive parameter values by converting all characters to asterisks (*). This feature is designed for template parameters that contain sensitive information like passwords, API keys, or other secrets.

> **Note**: This feature is purely cosmetic and does not provide any security. The actual parameter values are still transmitted and stored normally. This is only meant to hide the characters visually in the UI.

## Usage

The `mask_input` option can be applied to parameters with `form_type` of `input` or `textarea`. Add it to the `styling` block of your parameter definition:

```hcl
variable "api_key" {
  description = "API key for external service"
  type        = string
  sensitive   = true
  
  validation {
    condition     = length(var.api_key) > 0
    error_message = "API key cannot be empty."
  }
}

resource "coder_parameter" "api_key" {
  name         = "api_key"
  display_name = "API Key"
  description  = "Enter your API key for the external service"
  type         = "string"
  form_type    = "input"
  mutable      = true
  
  styling = {
    mask_input  = true
    placeholder = "Enter your API key"
  }
}
```

## Examples

### Masked Input Field

```hcl
resource "coder_parameter" "database_password" {
  name         = "database_password"
  display_name = "Database Password"
  description  = "Password for database connection"
  type         = "string"
  form_type    = "input"
  mutable      = true
  
  styling = {
    mask_input = true
  }
}
```

### Masked Textarea Field

```hcl
resource "coder_parameter" "private_key" {
  name         = "private_key"
  display_name = "Private Key"
  description  = "Private key for SSH access"
  type         = "string"
  form_type    = "textarea"
  mutable      = true
  
  styling = {
    mask_input  = true
    placeholder = "Paste your private key here"
  }
}
```

### Complete Example with Multiple Sensitive Parameters

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

variable "username" {
  description = "Username for the service"
  type        = string
}

variable "password" {
  description = "Password for the service"
  type        = string
  sensitive   = true
}

variable "ssl_certificate" {
  description = "SSL certificate content"
  type        = string
  sensitive   = true
}

resource "coder_parameter" "username" {
  name         = "username"
  display_name = "Username"
  description  = "Enter your username"
  type         = "string"
  form_type    = "input"
  mutable      = true
  default_value = var.username
}

resource "coder_parameter" "password" {
  name         = "password"
  display_name = "Password"
  description  = "Enter your password"
  type         = "string"
  form_type    = "input"
  mutable      = true
  default_value = var.password
  
  styling = {
    mask_input  = true
    placeholder = "Enter your password"
  }
}

resource "coder_parameter" "ssl_certificate" {
  name         = "ssl_certificate"
  display_name = "SSL Certificate"
  description  = "Paste your SSL certificate"
  type         = "string"
  form_type    = "textarea"
  mutable      = true
  default_value = var.ssl_certificate
  
  styling = {
    mask_input  = true
    placeholder = "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
  }
}
```

## User Interface Behavior

When `mask_input` is enabled:

1. **Masked Display**: All characters in the input field are displayed as asterisks (*)
2. **Show/Hide Toggle**: A eye icon button appears in the top-right corner of the field
   - Click the eye icon to reveal the actual text
   - Click again to hide it back to asterisks
3. **Normal Functionality**: The field works normally for typing, copying, and pasting
4. **Form Submission**: The actual unmasked value is submitted with the form

## Limitations and Considerations

- **No Security**: This feature provides no actual security - it's purely visual
- **Number Fields**: Masking is automatically disabled for `number` type parameters
- **Accessibility**: Screen readers will still read the actual values, not the masked version
- **Development**: Use in conjunction with Terraform's `sensitive = true` for variables that contain secrets

## Best Practices

1. **Combine with Sensitive Variables**: Always mark sensitive parameters with `sensitive = true` in your Terraform variables
2. **Use Descriptive Placeholders**: Provide helpful placeholder text to guide users
3. **Validate Input**: Add appropriate validation rules for sensitive parameters
4. **Documentation**: Clearly document what sensitive information is being collected

```hcl
variable "api_token" {
  description = "API token for external service (keep this secret!)"
  type        = string
  sensitive   = true  # This prevents the value from appearing in Terraform logs
  
  validation {
    condition     = can(regex("^[A-Za-z0-9]{32,}$", var.api_token))
    error_message = "API token must be at least 32 alphanumeric characters."
  }
}

resource "coder_parameter" "api_token" {
  name         = "api_token"
  display_name = "API Token"
  description  = "Enter your API token (this will be hidden for security)"
  type         = "string"
  form_type    = "input"
  mutable      = true
  default_value = var.api_token
  
  styling = {
    mask_input  = true
    placeholder = "Enter your 32+ character API token"
  }
}
```
