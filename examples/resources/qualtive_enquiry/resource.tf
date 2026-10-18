resource "qualtive_enquiry" "contact" {
  container    = "acme"
  workspace_id = 17
  slug         = "contact"
  name = {
    en = "Contact us"
    sv = "Kontakta oss"
  }

  pages = jsonencode([{
    id = 1
    content = [
      { type = "title", id = 1, text = { en = "How can we help?", sv = "Hur kan vi hjälpa till?" } },
      { type = "text", id = 2, storageTarget = { type = "text" } },
      { type = "contactDetails", id = 3, title = { en = "Your email", sv = "Din e-post" } },
    ]
  }])

  submitted_pages = jsonencode([{
    id         = 1
    conditions = []
    content    = [{ type = "title", id = 1, text = { en = "Thanks, we'll be in touch!", sv = "Tack, vi hör av oss!" } }]
  }])
}
