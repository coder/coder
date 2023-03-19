# Feature stages

Some Coder features are released as Alpha or Experimental.

## Alpha features

Alpha features are enabled in all Coder deployments but the feature is subject to change, or even be removed. Breaking changes may not be documented in the changelog. In most cases, features will only stay in alpha for 1 month.

We recommend using [GitHub issues](https://github.com/coder/coder/issues) to leave feedback and get support for alpha features.

## Experimental features

These features are disabled by default, and not recommended for use in production as they may cause performance or stability issues. In most cases, features will only stay in experimental for 1-2 weeks of internal testing.

```yaml
# Enable all experimental features
coder server --experiments=*

# Enable multiple experimental features
coder server --experiments=feature1,feature2

# Alternatively, use the `CODER_EXPERIMENTS` environment variable.
```

Visit `https://<your-coder-url>/api/v2/experiments` to see which experimental features are available for your deployment.
