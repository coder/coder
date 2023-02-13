# How to use NGINX as a reverse-proxy with LetsEncrypt

## Requirements

1. Start a Coder deployment with a wildcard subdomain. See [this guide](https://coder.com/docs/v2/latest/admin/configure#wildcard-access-url) for more information.

2. You'll need a subdomain and the a wildcard subdomain configured that resolves to server's public ip.

   > For example, to use `coder.example.com` as your subdomain, configure `coder.example.com` and `*.coder.example.com` to point to your server's public ip. This can be done by adding A records in your DNS provider's dashboard.

3. Install NGINX (assuming you're on Debian/Ubuntu):

   ```console
   sudo apt install nginx
   ```

4. Stop NGINX service:

   ```console
   sudo systemctl stop nginx
   ```

## Adding Coder deployment subdomain

> This example assumes Coder is running locally on `127.0.0.1:3000` for the subdomain `YOUR_SUBDOMAIN` e.g. `coder.example.com`.

1. Create NGINX configuration for this app:

   ```console
   sudo touch /etc/nginx/sites-available/YOUR_SUBDOMAIN
   ```

2. Activate this file:

   ```console
   sudo ln -s /etc/nginx/sites-available/YOUR_SUBDOMAIN /etc/nginx/sites-enabled/YOUR_SUBDOMAIN
   ```

## Install and configure LetsEncrypt Certbot

1. Install LetsEncrypt Certbot: Refer to the [CertBot documentation](https://certbot.eff.org/instructions?ws=other&os=pip&tab=wildcard)

## Create DNS provider credentials

1. Create an API token for the DNS provider you're using: e.g [CloudFlare](https://dash.cloudflare.com/profile/api-tokens) with the following permissions:

   - Zone - DNS - Edit

2. Create a file in `.secrets/certbot/cloudflare.ini` with the following content:

   ```ini
   dns_cloudflare_api_token = YOUR_API_TOKEN
   ```

3. Set the correct permissions:

   ```console
   sudo chmod 600 ~/.secrets/certbot/cloudflare.ini
   ```

## Create the certificate

1. Create the wildcard certificate:

   ```console
   sudo certbot certonly --dns-cloudflare --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini -d coder.example.com -d *.coder.example.com
   ```

## Configure nginx

1. Edit the file with:

   ```console
   sudo nano /etc/nginx/sites-available/YOUR_SUBDOMAIN
   ```

2. Add the following content:

   ```nginx
   server {
       server_name YOUR_SUBDOMAIN *.YOUR_SUBDOMAIN;

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
       ssl_certificate /etc/letsencrypt/live/YOUR_SUBDOMAIN/fullchain.pem;
       ssl_certificate_key /etc/letsencrypt/live/YOUR_SUBDOMAIN/privkey.pem;

       location / {
           proxy_pass  http://127.0.0.1:3000; # Change this to your coder deployment port default is 3000
           proxy_http_version 1.1;
           proxy_set_header Upgrade $http_upgrade;
           proxy_set_header Host $host;
           proxy_set_header X-Real-IP $remote_addr;
           proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
           proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;
           add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
       }
   }
   ```

> Don't forget to change:
>
> - `YOUR_SUBDOMAIN` by your (sub)domain e.g. `coder.example.com`

## Refresh certificates automatically

1. Create a new file in `/etc/cron.weekly`:

   ```console
   sudo touch /etc/cron.weekly/certbot
   ```

2. Make it executable:

   ```console
   sudo chmod +x /etc/cron.weekly/certbot
   ```

3. And add this code:

   ```sh
   #!/bin/sh
   sudo certbot renew -q
   ```

## Restart NGINX

- `sudo systemctl restart nginx`

And that's it, you should now be able to access Coder at `https://YOUR_SUBDOMAIN`!
