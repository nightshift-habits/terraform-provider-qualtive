terraform {
  required_providers {
    qualtive = {
      source  = "nightshift-habits/qualtive"
      version = "~> 0.1"
    }
  }
}

# The API key is read from QUALTIVE_API_KEY when api_key is not set.
provider "qualtive" {}
