# How to use NGINX as a reverse-proxy with LetsEncrypt

## Requirements

1. Start a Coder deployment and be sure to set the following
   [configuration values](../admin/setup/index.md):

   ```env
   CODER_HTTP_ADDRESS=127.0.0.1:3000
   CODER_ACCESS_URL=https://coder.example.com
   CODER_WILDCARD_ACCESS_URL=*.coder.example.com
   ```

   Throughout the guide, be sure to replace `coder.example.com` with the domain
   you intend to use with Coder.

2. Configure your DNS provider to point your coder.example.com and
   \*.coder.example.com to your server's public IP address.

   > For example, to use `coder.example.com` as your subdomain, configure
   > `coder.example.com` and `*.coder.example.com` to point to your server's
   > public ip. This can be done by adding A records in your DNS provider's
   > dashboard.

3. Install NGINX (assuming you're on Debian/Ubuntu):

   ```shell
   sudo apt install nginx
   ```

4. Stop NGINX service:

   ```shell
   sudo systemctl stop nginx
   ```

## Adding Coder deployment subdomain

This example assumes Coder is running locally on `127.0.0.1:3000` and that
you're using `coder.example.com` as your subdomain.

1. Create NGINX configuration for this app:

   ```shell
   sudo touch /etc/nginx/sites-available/coder.example.com
   ```

2. Activate this file:

   ```shell
   sudo ln -s /etc/nginx/sites-available/coder.example.com /etc/nginx/sites-enabled/coder.example.com
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

## Configure nginx

1. Edit the file with:

   ```shell
   sudo nano /etc/nginx/sites-available/coder.example.com
   ```

2. Add the following content:

   ```nginx
   server {
       server_name coder.example.com *.coder.example.com;

       # HTTP configuration
       listen 80;
       listen [::]:80;

       # HTTP to HTTPS
       if ($scheme != "https") {
           return 301 https://$host$request_uri;
       }

       # HTTPS configuration
       listen [::]:443 ssl ipv6only=on;
       listen 443 ssl;
       ssl_certificate /etc/letsencrypt/live/coder.example.com/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/coder.example.com/privkey.pem;

       location / {
           proxy_pass  http://127.0.0.1:3000; # Change this to your coder deployment port default is 3000
           proxy_http_version 1.1;
           proxy_set_header Upgrade $http_upgrade;
           proxy_set_header Connection upgrade;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;
           add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
       }
   }
   ```

   > Don't forget to change: `coder.example.com` by your (sub)domain

3. Test the configuration:

   ```shell
   sudo nginx -t
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

## Restart NGINX

```shell
sudo systemctl restart nginx
```

And that's it, you should now be able to access Coder at your sub(domain) e.g.
`https://coder.example.com`.
