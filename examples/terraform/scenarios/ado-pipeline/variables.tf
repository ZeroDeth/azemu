variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources"
  type        = string
  default     = "azemu-ado"
}

variable "ado_org_url" {
  description = "Azure DevOps organisation URL (used as the OIDC issuer for federation)"
  type        = string
  default     = "http://127.0.0.1:4569"
}
