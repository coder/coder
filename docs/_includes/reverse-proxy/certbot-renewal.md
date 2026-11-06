1. Create a new file in `/etc/cron.weekly`:

   ```sh
   sudo touch /etc/cron.weekly/certbot
   ```

2. Make it executable:

   ```sh
   sudo chmod +x /etc/cron.weekly/certbot
   ```

3. And add this code:

   ```sh
   #!/bin/sh
   sudo certbot renew -q
   ```
