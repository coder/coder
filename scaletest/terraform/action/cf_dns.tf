resource "cloudflare_record" "coder" {
  for_each = local.deployments
  zone_id  = var.cloudflare_zone_id
  name     = each.value.subdomain
  content  = google_compute_address.coder[each.key].address
  type     = "A"
  ttl      = 3600
}
