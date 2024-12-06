resource "cloudflare_record" "coder" {
  zone_id = var.cloudflare_zone_id
  name    = local.coder_subdomain
  content = google_compute_address.coder["primary"].address
  type    = "A"
  ttl     = 3600
}

resource "cloudflare_record" "coder_europe" {
  zone_id = var.cloudflare_zone_id
  name    = local.coder_europe_subdomain
  content = google_compute_address.coder["europe"].address
  type    = "A"
  ttl     = 3600
}
