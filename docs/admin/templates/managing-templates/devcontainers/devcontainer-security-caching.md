# Dev container security and caching

Ensure Envbuilder can only pull pre-approved images and artifacts by configuring
it with your existing HTTP proxies, firewalls, and artifact managers.

## Configure registry authentication

You may need to authenticate to your container registry, such as Artifactory, or
Git provider such as GitLab, to use Envbuilder. See the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/container-registry-auth.md)
for more information.

## Layer and image caching

To improve build times, dev containers can be cached. There are two main forms
of caching:

- **Layer caching**

  - Caches individual layers and pushes them to a remote registry. When building
    the image, Envbuilder will check the remote registry for pre-existing layers
    These will be fetched and extracted to disk instead of building the layers
    from scratch.

- **Image caching**

  - Caches the entire image, skipping the build process completely (except for
    post-build
    [lifecycle scripts](./add-devcontainer.md#dev-container-lifecycle-scripts)).

Note that caching requires push access to a registry, and may require approval
from relevant infrastructure team(s).

Refer to the
[Envbuilder documentation](https://github.com/coder/envbuilder/blob/main/docs/caching.md)
for more information about Envbuilder and caching.

Visit the
[speed up templates](../../../../tutorials/best-practices/speed-up-templates.md)
best practice documentation for more ways that you can speed up build times.

### Image caching

To support resuming from a cached image, use the
[Envbuilder Terraform Provider](https://github.com/coder/terraform-provider-envbuilder)
in your template. The provider will:

1. Clone the remote Git repository,
1. Perform a "dry-run" build of the dev container in the same manner as
   Envbuilder would,
1. Check for the presence of a previously built image in the provided cache
   repository,
1. Output the image remote reference in SHA256 form, if it finds one.

The example templates listed above will use the provider if a remote cache
repository is provided.

If you are building your own Dev container template, you can consult the
[provider documentation](https://registry.terraform.io/providers/coder/envbuilder/latest/docs/resources/cached_image).
You may also wish to consult a
[documented example usage of the `envbuilder_cached_image` resource](https://github.com/coder/terraform-provider-envbuilder/blob/main/examples/resources/envbuilder_cached_image/envbuilder_cached_image_resource.tf).

## Next steps

- [Dev container releases and known issues](./devcontainer-releases-known-issues.md)
- [Dotfiles](../../../../user-guides/workspace-dotfiles.md)
