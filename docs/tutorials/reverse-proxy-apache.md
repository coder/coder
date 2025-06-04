# How to use Apache as a reverse-proxy with LetsEncrypt

## Requirements

1. Start a Coder deployment and be sure to set the following
   [configuration values](../admin/setup/index.md):

   ```env
   CODER_HTTP_ADDRESS=127.0.0.1:3000
   CODER_ACCESS_URL=https://coder.example.com
   CODER_WILDCARD_ACCESS_URL=*coder.example.com
   ```

   Throughout the guide, be sure to replace `coder.example.com` with the domain
   you intend to use with Coder.

2. Configure your DNS provider to point your coder.example.com and
   \*.coder.example.com to your server's public IP address.

   > For example, to use `coder.example.com` as your subdomain, configure
   > `coder.example.com` and `*.coder.example.com` to point to your server's
   > public ip. This can be done by adding A records in your DNS provider's
   > dashboard.

3. Install Apache (assuming you're on Debian/Ubuntu):

   ```shell
   sudo apt install apache2
   ```

4. Enable the following Apache modules:

   ```shell
   sudo a2enmod proxy
   sudo a2enmod proxy_http
   sudo a2enmod ssl
   sudo a2enmod rewrite
   ```

5. Stop Apache service and disable default site:

   ```shell
   sudo a2dissite 000-default.conf
   sudo systemctl stop apache2
   ```

## Install and configure LetsEncrypt Certbot

1. Install LetsEncrypt Certbot: Refer to the
   [CertBot documentation](https://certbot.eff.org/instructions?ws=apache&os=ubuntufocal&tab=wildcard).
   Be sure to pick the wildcard tab and select your DNS provider for
   instructions to install the necessary DNS plugin.

## Create DNS provider credentials

This example assumes you're using CloudFlare as your DNS provider. For other
providers, refer to the
[CertBot documentation](https://eff-certbot.readthedocs.io/en/stable/using.html#dns-plugins).

1. Create an API token for the DNS provider you're using: e.g.
   [CloudFlare](https://developers.cloudflare.com/fundamentals/api/get-started/create-token)
   with the following permissions:

   - Zone - DNS - Edit

2. Create a file in `.secrets/certbot/cloudflare.ini` with the following
   content:

   ```ini
   dns_cloudflare_api_token = YOUR_API_TOKEN
   ```

   ```shell
   mkdir -p ~/.secrets/certbot
   touch ~/.secrets/certbot/cloudflare.ini
   nano ~/.secrets/certbot/cloudflare.ini
   ```

3. Set the correct permissions:

   ```shell
   sudo chmod 600 ~/.secrets/certbot/cloudflare.ini
   ```

## Create the certificate

1. Create the wildcard certificate:

   ```shell
   sudo certbot certonly --dns-cloudflare --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini -d coder.example.com -d *.coder.example.com
   ```

## Configure Apache

This example assumes Coder is running locally on `127.0.0.1:3000` and that
you're using `coder.example.com` as your subdomain.

1. Create Apache configuration for Coder:

   ```shell
   sudo nano /etc/apache2/sites-available/coder.conf
   ```

2. Add the following content:

   ```apache
    # Redirect HTTP to HTTPS
    <VirtualHost *:80>
        ServerName coder.example.com
        ServerAlias *.coder.example.com
        Redirect permanent / https://coder.example.com/
    </VirtualHost>

    <VirtualHost *:443>
        ServerName coder.example.com
        ServerAlias *.coder.example.com
        ErrorLog ${APACHE_LOG_DIR}/error.log
        CustomLog ${APACHE_LOG_DIR}/access.log combined

        ProxyPass / http://127.0.0.1:3000/ upgrade=any # required for websockets
        ProxyPassReverse / http://127.0.0.1:3000/
        ProxyRequests Off
        ProxyPreserveHost On

        RewriteEngine On
        # Websockets are required for workspace connectivity
        RewriteCond %{HTTP:Connection} Upgrade [NC]
        RewriteCond %{HTTP:Upgrade} websocket [NC]
        RewriteRule /(.*) ws://127.0.0.1:3000/$1 [P,L]

        SSLCertificateFile /etc/letsencrypt/live/coder.example.com/fullchain.pem
        SSLCertificateKeyFile /etc/letsencrypt/live/coder.example.com/privkey.pem
    </VirtualHost>
   ```

   > Don't forget to change: `coder.example.com` by your (sub)domain

3. Enable the site:

   ```shell
   sudo a2ensite coder.conf
   ```

4. Restart Apache:

   ```shell
   sudo systemctl restart apache2
   ```

## Refresh certificates automatically

1. Create a new file in `/etc/cron.weekly`:

   ```shell
   sudo touch /etc/cron.weekly/certbot
   ```

2. Make it executable:

   ```shell
   sudo chmod +x /etc/cron.weekly/certbot
   ```

3. And add this code:

   ```shell
   #!/bin/sh
   sudo certbot renew -q
   ```

And that's it, you should now be able to access Coder at your sub(domain) e.g.
`https://coder.example.com`.
