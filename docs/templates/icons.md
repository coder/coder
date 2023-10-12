# Icons

---

Coder uses icons in several places, including ones that can be configured
throughout the app, or specified in your Terraform. They're specified by a URL,
which can be to an image hosted on a CDN of your own, or one of the icons that
come bundled with your Coder deployment.

- **Template Icons**:

  - Make templates and workspaces visually recognizable with a relevant or
    memorable icon

- [**Terraform**](https://registry.terraform.io/providers/coder/coder/latest/docs):

  - [`coder_app`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app#icon)
  - [`coder_parameter`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#icon)
    and
    [`option`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/parameter#nested-schema-for-option)
    blocks
  - [`coder_script`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script#icon)

  These can all be configured to use an icon by setting the `icon` field.

  ```terraform
  data "coder_parameter" "my_parameter" {
    icon = "/icon/coder.svg"

    option {
      icon = "/emojis/1f3f3-fe0f-200d-26a7-fe0f.png"
    }
  }
  ```

- [**Authentication Providers**](https://coder.com/docs/v2/latest/admin/external-auth):

  - Use icons for external authentication providers to make them recognizable

## Bundled icons

Coder includes icons for popular cloud providers and programming languages. You
can see all of the icons (or suggest new ones) in our repository on
[GitHub](https://github.com/coder/coder/tree/main/site/static/icon).

You can also view the entire list, with search, by navigating to /icons on your
Coder deployment. E.g. [https://coder.example.com/icons](#). This can be
particularly useful in airgapped deployments.

![The icon gallery](../images/icons-gallery.png)
