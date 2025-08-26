data "cloudflare_zone" "domain" {
  name = var.cloudflare_domain
}

resource "cloudflare_record" "coder" {
  for_each = local.deployments
  zone_id  = data.cloudflare_zone.domain.zone_id
  name     = "${each.value.subdomain}.${var.cloudflare_domain}"
  content  = google_compute_address.coder[each.key].address
  type     = "A"
  ttl      = 3600
}

resource "cloudflare_record" "coder_wildcard" {
  for_each = local.deployments
  zone_id  = data.cloudflare_zone.domain.id
  name     = each.value.wildcard_subdomain
  content  = cloudflare_record.coder[each.key].name
  type     = "CNAME"
  ttl      = 3600
}
