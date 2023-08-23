# Your first template

A common way to create a template is to begin with a starter template
then modify it for your needs. Coder makes this easy with starter
templates for popular development targets, like Docker, Kubernetes,
Azure, and so on. Once your template is up and running, you can edit
it in the Coder web page. Coder even handles versioning for you.

In this tutorial, you'll create your first template from the Docker
starter template. You'll need ssh access to a computer that has both
Docker and Coder installed on it.

To create a template, you'll work with both the Coder web page and the
command line. You only need to do this once to create a template.
After that, you can edit your template from the Coder web page.

## Log in to Coder

In your web browser, go to your Coder instance to log in.

Also, in a terminal, ssh into the computer where Docker and Coder are
installed, then log into Coder.

```console
marc@example:~$ coder login https://example-access-url.coder.app
Open the following in your browser:

        https://example-access-url.coder.app/cli-auth

> Paste your token here:
> Welcome to Coder, marc! You're authenticated.
marc@example:~$ 
```

## Choose a starter template

In the web browser, select **Templates** > **Starter Templates**.

![Starter Templates button](../images/templates/starter-templates.png)

In **Filter**, select **Docker** then select **Develop in Docker**.

![Choosing a starter template](../images/templates/develop-in-docker-template.png)

## Try out your new template


## Creating a template for Kubernetes

## Next steps

- [Anatomy of a template](./anatomy.md)
- [Setting up templates](./best-practices.md)
