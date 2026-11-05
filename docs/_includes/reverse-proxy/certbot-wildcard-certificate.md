1. Create the wildcard certificate:

   ```sh
   sudo certbot certonly --dns-cloudflare --dns-cloudflare-credentials ~/.secrets/certbot/cloudflare.ini -d coder.example.com -d *.coder.example.com
   ```
