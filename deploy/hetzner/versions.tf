terraform {
  required_version = ">= 1.7.0"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = ">= 1.58, < 2.0"
    }
  }
}

provider "hcloud" {
  token = var.hcloud_token
}
