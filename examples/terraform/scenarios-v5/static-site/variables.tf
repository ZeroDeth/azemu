variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources"
  type        = string
  default     = "azemusite"
}

variable "zone_name" {
  description = "DNS zone name for the static site"
  type        = string
  default     = "staticsite.local"
}
