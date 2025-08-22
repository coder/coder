data "cloudflare_zone" "domain" {
  name = var.cloudflare_domain
}

resource "cloudflare_record" "coder" {
  for_each = local.deployments
  zone_id  = data.cloudflare_zone.domain.zone_id
  name     = each.value.subdomain
  content  = google_compute_address.coder[each.key].address
  type     = "A"
  ttl      = 3600
}
