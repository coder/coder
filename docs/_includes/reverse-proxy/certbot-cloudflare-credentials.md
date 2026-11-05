This example assumes you're using CloudFlare as your DNS provider.
For other providers, refer to the [CertBot documentation](https://eff-certbot.readthedocs.io/en/stable/using.html#dns-plugins).

1. Create an API token for the DNS provider you're using, for example, [CloudFlare](https://developers.cloudflare.com/fundamentals/api/get-started/create-token) with the following permissions:

   - Zone - DNS - Edit

2. Create a file in `.secrets/certbot/cloudflare.ini` with the following content:

   ```ini
   dns_cloudflare_api_token = YOUR_API_TOKEN
   ```

   ```sh
   mkdir -p ~/.secrets/certbot
   touch ~/.secrets/certbot/cloudflare.ini
   nano ~/.secrets/certbot/cloudflare.ini
   ```

3. Set the correct permissions:

   ```sh
   sudo chmod 600 ~/.secrets/certbot/cloudflare.ini
   ```
