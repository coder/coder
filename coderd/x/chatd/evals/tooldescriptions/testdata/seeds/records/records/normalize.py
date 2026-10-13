"""Normalizers for imported customer record fields.

Each normalize_<field> takes the raw value and returns the cleaned value.
"""


def normalize_contact_channel(value):
    """Normalize the contact channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_alias(value):
    """Normalize the audit alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_source(value):
    """Normalize the shipping source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_tag(value):
    """Normalize the billing tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_label(value):
    """Normalize the delivery label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_tag(value):
    """Normalize the service tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_ref(value):
    """Normalize the contact ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_status(value):
    """Normalize the contact status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_zone(value):
    """Normalize the audit zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_zone(value):
    """Normalize the billing zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_title(value):
    """Normalize the marketing title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_handle(value):
    """Normalize the account handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_title(value):
    """Normalize the account title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_title(value):
    """Normalize the vendor title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_code(value):
    """Normalize the payment code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_segment(value):
    """Normalize the account segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_channel(value):
    """Normalize the risk channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_channel(value):
    """Normalize the backup channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_title(value):
    """Normalize the support title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_tag(value):
    """Normalize the contact tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_code(value):
    """Normalize the risk code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_group(value):
    """Normalize the backup group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_code(value):
    """Normalize the partner code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_tier(value):
    """Normalize the primary tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_status(value):
    """Normalize the backup status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_group(value):
    """Normalize the payment group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_source(value):
    """Normalize the region source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_zone(value):
    """Normalize the backup zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_note(value):
    """Normalize the marketing note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_note(value):
    """Normalize the audit note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_handle(value):
    """Normalize the service handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_tier(value):
    """Normalize the vendor tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_label(value):
    """Normalize the marketing label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_status(value):
    """Normalize the primary status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_group(value):
    """Normalize the legacy group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_label(value):
    """Normalize the audit label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_group(value):
    """Normalize the support group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_tier(value):
    """Normalize the region tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_segment(value):
    """Normalize the legacy segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_handle(value):
    """Normalize the payment handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_source(value):
    """Normalize the delivery source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_segment(value):
    """Normalize the risk segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_handle(value):
    """Normalize the primary handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_title(value):
    """Normalize the legacy title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_code(value):
    """Normalize the backup code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_title(value):
    """Normalize the partner title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_source(value):
    """Normalize the legacy source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_handle(value):
    """Normalize the risk handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_source(value):
    """Normalize the payment source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_status(value):
    """Normalize the delivery status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_handle(value):
    """Normalize the legacy handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_title(value):
    """Normalize the audit title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_status(value):
    """Normalize the support status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_channel(value):
    """Normalize the vendor channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_zone(value):
    """Normalize the shipping zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_zone(value):
    """Normalize the loyalty zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_channel(value):
    """Normalize the invoice channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_title(value):
    """Normalize the service title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_group(value):
    """Normalize the vendor group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_ref(value):
    """Normalize the legacy ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_note(value):
    """Normalize the legacy note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_handle(value):
    """Normalize the loyalty handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_status(value):
    """Normalize the audit status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_ref(value):
    """Normalize the sales ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_channel(value):
    """Normalize the service channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_alias(value):
    """Normalize the invoice alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_tag(value):
    """Normalize the invoice tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_tag(value):
    """Normalize the primary tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_segment(value):
    """Normalize the shipping segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_tag(value):
    """Normalize the vendor tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_group(value):
    """Normalize the sales group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_code(value):
    """Normalize the loyalty code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_title(value):
    """Normalize the backup title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_alias(value):
    """Normalize the marketing alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_channel(value):
    """Normalize the billing channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_alias(value):
    """Normalize the vendor alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_tier(value):
    """Normalize the account tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_segment(value):
    """Normalize the vendor segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_group(value):
    """Normalize the risk group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_title(value):
    """Normalize the payment title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_source(value):
    """Normalize the partner source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_note(value):
    """Normalize the sales note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_alias(value):
    """Normalize the region alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_note(value):
    """Normalize the billing note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_ref(value):
    """Normalize the primary ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_status(value):
    """Normalize the partner status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_alias(value):
    """Normalize the account alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_status(value):
    """Normalize the invoice status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_ref(value):
    """Normalize the marketing ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_tag(value):
    """Normalize the payment tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_zone(value):
    """Normalize the sales zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_label(value):
    """Normalize the service label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_title(value):
    """Normalize the contact title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_ref(value):
    """Normalize the support ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_label(value):
    """Normalize the primary label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_label(value):
    """Normalize the vendor label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_tag(value):
    """Normalize the risk tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_note(value):
    """Normalize the partner note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_group(value):
    """Normalize the partner group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_segment(value):
    """Normalize the audit segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_zone(value):
    """Normalize the region zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_segment(value):
    """Normalize the service segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_note(value):
    """Normalize the service note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_tag(value):
    """Normalize the support tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_source(value):
    """Normalize the vendor source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_status(value):
    """Normalize the region status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_note(value):
    """Normalize the delivery note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_ref(value):
    """Normalize the region ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_code(value):
    """Normalize the audit code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_title(value):
    """Normalize the billing title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_code(value):
    """Normalize the vendor code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_alias(value):
    """Normalize the backup alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_segment(value):
    """Normalize the delivery segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_source(value):
    """Normalize the billing source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_note(value):
    """Normalize the loyalty note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_status(value):
    """Normalize the marketing status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_channel(value):
    """Normalize the loyalty channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_code(value):
    """Normalize the billing code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_note(value):
    """Normalize the support note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_ref(value):
    """Normalize the billing ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_label(value):
    """Normalize the risk label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_note(value):
    """Normalize the payment note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_tier(value):
    """Normalize the billing tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_handle(value):
    """Normalize the delivery handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_channel(value):
    """Normalize the legacy channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_alias(value):
    """Normalize the support alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_group(value):
    """Normalize the billing group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_group(value):
    """Normalize the service group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_ref(value):
    """Normalize the delivery ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_segment(value):
    """Normalize the partner segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_segment(value):
    """Normalize the loyalty segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_channel(value):
    """Normalize the shipping channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_code(value):
    """Normalize the support code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_note(value):
    """Normalize the invoice note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_group(value):
    """Normalize the invoice group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_zone(value):
    """Normalize the invoice zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_channel(value):
    """Normalize the delivery channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_segment(value):
    """Normalize the billing segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_handle(value):
    """Normalize the sales handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_ref(value):
    """Normalize the audit ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_ref(value):
    """Normalize the backup ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_tag(value):
    """Normalize the account tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_label(value):
    """Normalize the support label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_segment(value):
    """Normalize the marketing segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_status(value):
    """Normalize the service status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_label(value):
    """Normalize the billing label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_group(value):
    """Normalize the audit group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_zone(value):
    """Normalize the primary zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_zone(value):
    """Normalize the risk zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_tag(value):
    """Normalize the partner tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_alias(value):
    """Normalize the billing alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_code(value):
    """Normalize the marketing code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_source(value):
    """Normalize the audit source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_label(value):
    """Normalize the payment label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_code(value):
    """Normalize the legacy code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_ref(value):
    """Normalize the shipping ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_code(value):
    """Normalize the sales code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_tier(value):
    """Normalize the legacy tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_zone(value):
    """Normalize the account zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_label(value):
    """Normalize the account label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_label(value):
    """Normalize the region label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_tier(value):
    """Normalize the payment tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_label(value):
    """Normalize the invoice label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_handle(value):
    """Normalize the region handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_tier(value):
    """Normalize the loyalty tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_channel(value):
    """Normalize the primary channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_handle(value):
    """Normalize the support handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_source(value):
    """Normalize the account source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_channel(value):
    """Normalize the payment channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_alias(value):
    """Normalize the sales alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_alias(value):
    """Normalize the risk alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_handle(value):
    """Normalize the marketing handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_status(value):
    """Normalize the account status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_channel(value):
    """Normalize the account channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_title(value):
    """Normalize the risk title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_alias(value):
    """Normalize the contact alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_title(value):
    """Normalize the sales title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_status(value):
    """Normalize the legacy status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_alias(value):
    """Normalize the delivery alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_tier(value):
    """Normalize the sales tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_ref(value):
    """Normalize the service ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_ref(value):
    """Normalize the vendor ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_segment(value):
    """Normalize the contact segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_code(value):
    """Normalize the account code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_segment(value):
    """Normalize the region segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_handle(value):
    """Normalize the billing handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_segment(value):
    """Normalize the backup segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_channel(value):
    """Normalize the marketing channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_group(value):
    """Normalize the account group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_alias(value):
    """Normalize the payment alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_source(value):
    """Normalize the primary source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_status(value):
    """Normalize the sales status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_title(value):
    """Normalize the invoice title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_note(value):
    """Normalize the risk note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_tier(value):
    """Normalize the service tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_tag(value):
    """Normalize the backup tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_tier(value):
    """Normalize the invoice tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_zone(value):
    """Normalize the service zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_code(value):
    """Normalize the shipping code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_label(value):
    """Normalize the shipping label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_handle(value):
    """Normalize the invoice handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_ref(value):
    """Normalize the risk ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_label(value):
    """Normalize the partner label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_title(value):
    """Normalize the primary title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_alias(value):
    """Normalize the loyalty alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_note(value):
    """Normalize the shipping note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_source(value):
    """Normalize the risk source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_title(value):
    """Normalize the delivery title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_label(value):
    """Normalize the loyalty label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_zone(value):
    """Normalize the partner zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_note(value):
    """Normalize the vendor note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_tier(value):
    """Normalize the backup tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_source(value):
    """Normalize the invoice source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_label(value):
    """Normalize the sales label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_handle(value):
    """Normalize the backup handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_tier(value):
    """Normalize the delivery tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_source(value):
    """Normalize the service source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_channel(value):
    """Normalize the support channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_zone(value):
    """Normalize the support zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_code(value):
    """Normalize the delivery code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_alias(value):
    """Normalize the service alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_ref(value):
    """Normalize the invoice ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_billing_status(value):
    """Normalize the billing status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_tag(value):
    """Normalize the sales tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_handle(value):
    """Normalize the partner handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_group(value):
    """Normalize the contact group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_source(value):
    """Normalize the sales source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_segment(value):
    """Normalize the payment segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_source(value):
    """Normalize the contact source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_note(value):
    """Normalize the region note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_channel(value):
    """Normalize the partner channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_handle(value):
    """Normalize the audit handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_alias(value):
    """Normalize the shipping alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_handle(value):
    """Normalize the vendor handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_tier(value):
    """Normalize the audit tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_name(value):
    """Normalize a person's name: collapse whitespace and title-case it.

    Empty or missing names become "unknown".
    """
    if value is None:
        return "unknown"
    parts = str(value).split(" ")
    return " ".join(part.capitalize() for part in parts)


def normalize_loyalty_group(value):
    """Normalize the loyalty group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_ref(value):
    """Normalize the payment ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_source(value):
    """Normalize the backup source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_tier(value):
    """Normalize the marketing tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_ref(value):
    """Normalize the partner ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_tier(value):
    """Normalize the partner tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_status(value):
    """Normalize the risk status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_source(value):
    """Normalize the support source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_handle(value):
    """Normalize the shipping handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_note(value):
    """Normalize the primary note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_tag(value):
    """Normalize the delivery tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_invoice_segment(value):
    """Normalize the invoice segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_source(value):
    """Normalize the loyalty source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_ref(value):
    """Normalize the loyalty ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_title(value):
    """Normalize the loyalty title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_backup_label(value):
    """Normalize the backup label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_audit_channel(value):
    """Normalize the audit channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_channel(value):
    """Normalize the region channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_channel(value):
    """Normalize the sales channel field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_tier(value):
    """Normalize the support tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_sales_segment(value):
    """Normalize the sales segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_status(value):
    """Normalize the vendor status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_group(value):
    """Normalize the marketing group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_risk_tier(value):
    """Normalize the risk tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_code(value):
    """Normalize the region code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_loyalty_status(value):
    """Normalize the loyalty status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_note(value):
    """Normalize the contact note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_tag(value):
    """Normalize the legacy tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_payment_status(value):
    """Normalize the payment status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_ref(value):
    """Normalize the account ref field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_zone(value):
    """Normalize the legacy zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_account_note(value):
    """Normalize the account note field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_vendor_zone(value):
    """Normalize the vendor zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_code(value):
    """Normalize the primary code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_segment(value):
    """Normalize the primary segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_service_code(value):
    """Normalize the service code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_support_segment(value):
    """Normalize the support segment field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_group(value):
    """Normalize the primary group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_group(value):
    """Normalize the shipping group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_group(value):
    """Normalize the region group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_status(value):
    """Normalize the shipping status field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_delivery_group(value):
    """Normalize the delivery group field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_label(value):
    """Normalize the contact label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_title(value):
    """Normalize the region title field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_primary_alias(value):
    """Normalize the primary alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_code(value):
    """Normalize the contact code field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_zone(value):
    """Normalize the marketing zone field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_legacy_label(value):
    """Normalize the legacy label field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_partner_alias(value):
    """Normalize the partner alias field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_handle(value):
    """Normalize the contact handle field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_tag(value):
    """Normalize the marketing tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_marketing_source(value):
    """Normalize the marketing source field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_contact_tier(value):
    """Normalize the contact tier field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_shipping_tag(value):
    """Normalize the shipping tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


def normalize_region_tag(value):
    """Normalize the region tag field."""
    if value is None:
        return None
    text = str(value).strip()
    if not text:
        return None
    return text.lower()


NORMALIZERS = {name[len("normalize_"):]: fn for name, fn in globals().items() if name.startswith("normalize_")}


def normalize_record(record):
    """Apply every known normalizer to a record dict."""
    return {key: NORMALIZERS[key](value) if key in NORMALIZERS else value for key, value in record.items()}
