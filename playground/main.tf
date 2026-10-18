terraform {
  required_providers {
    qualtive = {
      source = "nightshift-habits/qualtive"
    }
  }
}

variable "endpoint" {
  type    = string
  default = "http://localhost:8093"
}

variable "container" {
  type    = string
  default = "qualtive"
}

variable "workspace_id" {
  type = number
}

provider "qualtive" {
  endpoint = var.endpoint
}

resource "qualtive_enquiry" "playground" {
  container    = var.container
  workspace_id = var.workspace_id
  slug         = "terraform-playground"
  name         = { en = "Terraform playground" }

  pages = jsonencode([{
    id = 1
    content = [
      { type = "title", id = 1, text = { en = "How is the provider working out?" } },
      { type = "score", id = 2, scoreType = "smilies5" },
      { type = "text", id = 3, storageTarget = { type = "text" } },
    ]
  }])
}
