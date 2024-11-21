resource "cloudflare_record" "coder" {
  zone_id = var.cloudflare_zone_id
  name    = local.coder_subdomain
  content = google_compute_address.coder["primary"].address
  type    = "A"
  ttl     = 3600
}
