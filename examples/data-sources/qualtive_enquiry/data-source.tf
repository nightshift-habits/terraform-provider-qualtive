data "qualtive_enquiry" "contact" {
  container = "acme"
  slug      = "contact"
}

output "contact_enquiry_id" {
  value = data.qualtive_enquiry.contact.id
}
