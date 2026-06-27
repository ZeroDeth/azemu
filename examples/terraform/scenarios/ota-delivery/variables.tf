variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources. The storage account name is the prefix plus 'sa' and must stay <= 24 lowercase alphanumerics; it must also be registered in docker-compose.yml AZURITE_ACCOUNTS."
  type        = string
  default     = "azemuotad"
}

variable "runtime_version" {
  description = "OTA runtime version, the first path dimension under the container"
  type        = string
  default     = "1.0.0"
}

variable "channel" {
  description = "Release channel, the second path dimension"
  type        = string
  default     = "PRODUCTION"
}

variable "platform" {
  description = "Target platform, the third path dimension"
  type        = string
  default     = "android"
}
