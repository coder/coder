# Converting an OpenAPI/Swagger file to Markdown with the Widdershins CLI

The simplest way to convert an OpenAPI or Swagger file to Markdown with Widdershins is to use the Widdershins command-line interface (CLI).
For a more powerful way to use Widdershins, see [Converting an OpenAPI/Swagger file to Markdown with the Widdershins JavaScript interface](ConvertingFilesBasicJS.md).

## Prerequisites

- Install NodeJS and Node Package Manager (NPM).
See [nodejs.org](https://nodejs.org/).
- Install Widdershins and its dependencies.
The easiest way is to use NPM to install Widdershins globally so you can use it with the command line from any folder.
From a terminal window, run this command:
```shell
npm install -g widdershins
```

## Converting files on the command line

1. Get an OpenAPI 3.0 or Swagger 2.0 file.
To test the process, you can use the pet store sample here: https://petstore.swagger.io/v2/swagger.json
The file must parse as a valid OpenAPI or Swagger file.
1. Assemble the options that you want to use to convert the file.
These options are listed in the [README.md](https://github.com/Mermade/widdershins#options) file.

  Note that some of these options are useful only if you intend to take the Markdown output from Widdershins and convert it to HTML with [Shins](https://github.com/Mermade/shins).
  Other options are not usable from the command line.

  For example, the `language_tabs` option specifies a list of one or more languages to generate examples in, each with an ID and display name.
  You can generate examples in Ruby and Python with the command-line option `--language_tabs 'ruby:Ruby' 'python:Python'`.
1. Optional: Put the options in an environment file for easier reuse.
Environment files contain the options for the conversion in JSON format.
For environment files, use the JavaScript parameter name from the [README.md](https://github.com/Mermade/widdershins#options) file, not the CLI parameter name.
For example:
```json
{
"language_tabs": [{ "python": "Python" }, { "ruby": "Ruby" }]
}
```
1. Convert the file with the `widdershins` command, specify the name of the OpenAPI or Swagger file, and specify the name of the output file with the `-o` option.
Include the options in the command or specify the name of the environment file with the `--environment` option, as in this example:
```shell
widdershins --environment env.json swagger.json -o myOutput.md
```

Now you can use the Markdown file in your documentation or use a tool such as Shins to convert it to HTML.
