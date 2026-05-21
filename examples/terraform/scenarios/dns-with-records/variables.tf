variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources"
  type        = string
  default     = "azemu-dns"
}

variable "zone_name" {
  description = "DNS zone name (must be a valid domain name)"
  type        = string
  default     = "example.internal"
}
