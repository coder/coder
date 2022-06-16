# PostgreSQL

Coder ships with a built-in PostgreSQL database, but if you'd like to set up and
use your own, refer to the following instructions.

For in-depth information, please see the [PostgreSQL documentation](https://www.postgresql.org/docs/current/tutorial-start.html).

## PostgreSQL on macOS with Homebrew

1. Install with Homebrew:

    ```console
    brew install postgres
    ```

1. Start PostgreSQL with brew

    ```console
    brew services start postgresql
    ```

1. Connect to PostgreSQL:

    ```console
    psql postgres
    ```

1. Create the `coderuser` role:

    ```console
    create role coder-user with login;
    ```

1. Create a database called `coder` and assign the owner:

    ```console
    create database coder owner coderuser;
    ```

1. Set the password for `coderuser`:

    ```console
    \password coder # enter password when prompted
    ```

1. Assign rights to the database to your user:

    ```console
    grant all privileges on database coder to coderuser;
    ```

## PostgreSQL on Debian/Ubuntu

1. Install PostgreSQL:

    ```console
    sudo apt-get install -y postgresql
    ```

1. Start PostgreSQL:

    ```console
    sudo systemctl start postgresql
    ```

1. Connect to PostgreSQL:

    ```console
    sudo -u postgresql psql
    ```

1. Create the `coderuser` role:

    ```console
    create role coderuser with login;
    ```

1. Create a database called `coder` and assign the owner:

    ```console
    create database coder owner coder;
    ```

1. Set the password for `coderuser`:

    ```console
    \password coder # enter password when prompted
    ```

1. Assign rights to the database to your user:

    ```console
    grant all privileges on database coder to coderuser;
    ```

## Using your PostgreSQL database with Coder

To use your Postgres database with Coder, provide the `CODER_PG_CONNECTION_URL`
variable:

```console
postgresql://[user[:password]@][networkLocation][:port][/dbname][?param1=value1&...]
```

Append to `coder server` to start your deployment. For example:

```console
CODER_PG_CONNECTION_URL="postgres://<databaseUsername>@0.0.0.0/<databaseName>?sslmode=disable&password=<password>" \
    coder server -a 0.0.0.0:3000 --verbose
```
