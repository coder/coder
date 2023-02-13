# How to use NGINX as a reverse-proxy with LetsEncrypt

## Requirements

1. You'll need a subdomain and the a wildcard subdomain configured that resolves to server.
2. Install **nginx** (assuming you're on Debian/Ubuntu):

- `sudo apt install nginx`

3. Stop **nginx** :

- `sudo service stop nginx`

## Adding Coder deployment subdomain

> This example assumes Coder is running locally on `127.0.0.1:3000` for the subdomain `YOUR_SUBDOMAIN` e.g. `coder.example.com`.

- Create NGINX configuration for this app : `sudo touch /etc/nginx/sites-available/YOUR_SUBDOMAIN`

- Activate this file : `sudo ln -s /etc/nginx/sites-available/YOUR_SUBDOMAIN /etc/nginx/sites-enabled/YOUR_SUBDOMAIN`

## Install and configure LetsEncrypt Certbot

Install LetsEncrypt Certbot: Refer to the [CertBot documentation](https://certbot.eff.org/instructions?ws=other&os=pip&tab=wildcard)

## Create dns provider credentials

- Create an API token for the dns provider you're using : e.g cloudflare [here](https://dash.cloudflare.com/profile/api-tokens) with the following permissions :
  - Zone - DNS - Edit
- Create a file in `.secrets/certbot/cloudflare.ini` with the following content :
  - `dns_cloudflare_api_token = YOUR_API_TOKEN`

## Create the certificate

- Create the wildcard certificate :

```console
sudo certbot certonly --dns-cloudflare --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini -d coder.example.com *.coder.example.com
```

## Configure nginx

Edit the file with : `sudo nano /etc/nginx/sites-available/YOUR_SUBDOMAIN` and add the following content :

```nginx
server {
    server_name YOUR_SUBDOMAIN;

    # HTTP configuration
    listen 80;
    listen [::]:80;

    # HTTP to HTTPS
    if ($scheme != "https") {
        return 301 https://$host$request_uri;
    } # managed by Certbot

    # HTTPS configuration
    listen [::]:443 ssl ipv6only=on; # managed by Certbot
    listen 443 ssl; # managed by Certbot
    ssl_certificate /etc/letsencrypt/live/YOUR_SUBDOMAIN/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/YOUR_SUBDOMAIN/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem; # managed by Certbot

    location / {
        proxy_pass  http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $server_name;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $http_x_forwarded_proto;
        add_header Strict-Transport-Security "max-age=15552000; includeSubDomains" always;
    }
}
```

> Don't forget to change :
>
> - `YOUR_SUBDOMAIN` by your (sub)domain e.g. `coder.example.com`
> - the port and ip in `proxy_pass` if applicable

## Automatic certificates refreshing

- Create a new file in `/etc/cron.weekly` : `sudo touch /etc/cron.weekly/certbot`
- Make it executable : `sudo chmod +x /etc/cron.weekly/certbot`
- And add this code :

```sh
#!/bin/sh
sudo certbot renew -q
```

## Restart NGINX

- `sudo service nginx restart`

And that's it, you should now be able to access coder via `https://YOUR_SUBDOMAIN` !
