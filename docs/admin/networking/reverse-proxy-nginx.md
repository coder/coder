---
title: How to use NGINX as a reverse proxy with Let's Encrypt
---

## Requirements

1. Start a Coder deployment and be sure to set the following [configuration values](../setup/index.md):

   ```dotenv
   CODER_HTTP_ADDRESS=127.0.0.1:3000
   CODER_ACCESS_URL=https://coder.example.com
   CODER_WILDCARD_ACCESS_URL=*.coder.example.com
   ```

   Throughout the guide, be sure to replace `coder.example.com` with the domain you intend to use with Coder.

2. Configure your DNS provider to point your coder.example.com and `*.coder.example.com` to your server's public IP address.

   > For example, to use `coder.example.com` as your subdomain, configure `coder.example.com` and `*.coder.example.com` to point to your server's public ip.
   > This can be done by adding A records in your DNS provider's dashboard.

3. Install NGINX (assuming you're on Debian/Ubuntu):

   ```sh
   sudo apt install nginx
   ```

4. Stop NGINX service:

   ```sh
   sudo systemctl stop nginx
   ```

<a id="adding-coder-deployment-subdomain"></a>

## Add the Coder deployment subdomain

This example assumes Coder is running locally on `127.0.0.1:3000` and that you're using `coder.example.com` as your subdomain.

1. Create NGINX configuration for this app:

   ```sh
   sudo touch /etc/nginx/sites-available/coder.example.com
   ```

2. Activate this file:

   ```sh
   sudo ln -s /etc/nginx/sites-available/coder.example.com /etc/nginx/sites-enabled/coder.example.com
   ```

## Install and configure LetsEncrypt Certbot

1. Install LetsEncrypt Certbot: Refer to the [CertBot documentation](https://certbot.eff.org/instructions?ws=nginx&os=ubuntufocal&tab=wildcard).

   Be sure to pick the wildcard tab and select your DNS provider for instructions to install the necessary DNS plugin.

## Create DNS provider credentials

<include>../../_includes/reverse-proxy/certbot-cloudflare-credentials.md</include>

## Create the certificate

<include>../../_includes/reverse-proxy/certbot-wildcard-certificate.md</include>

## Configure nginx

1. Edit the file with:

   ```sh
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
           proxy_set_header X-Forwarded-Proto $scheme;
           add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
       }
   }
   ```

   > Do not forget to change: `coder.example.com` by your (sub)domain

3. Test the configuration:

   ```sh
   sudo nginx -t
   ```

## Refresh certificates automatically

<include>../../_includes/reverse-proxy/certbot-renewal.md</include>

## Restart NGINX

```sh
sudo systemctl restart nginx
```

And that's it, you should now be able to access Coder at your sub(domain), for example, `https://coder.example.com`.
