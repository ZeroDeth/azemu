variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources. The storage account name is the prefix plus 'sa' and must stay <= 24 lowercase alphanumerics; it must also be registered in docker-compose.yml AZURITE_ACCOUNTS."
  type        = string
  default     = "azemuotad"

  validation {
    condition     = can(regex("^[a-z0-9]{1,22}$", var.prefix))
    error_message = "prefix must be 1-22 lowercase alphanumerics so the derived '${var.prefix}sa' storage account name stays valid."
  }
}

variable "runtime_version" {
  description = "OTA runtime version, the first path dimension under the container"
  type        = string
  default     = "1.0.0"

  validation {
    condition     = can(regex("^[^/]+$", var.runtime_version))
    error_message = "runtime_version must be non-empty and must not contain '/', since it is a fixed blob path segment."
  }
}

variable "channel" {
  description = "Release channel, the second path dimension"
  type        = string
  default     = "PRODUCTION"

  validation {
    condition     = can(regex("^[^/]+$", var.channel))
    error_message = "channel must be non-empty and must not contain '/', since it is a fixed blob path segment."
  }
}

variable "platform" {
  description = "Target platform, the third path dimension"
  type        = string
  default     = "android"

  validation {
    condition     = can(regex("^[^/]+$", var.platform))
    error_message = "platform must be non-empty and must not contain '/', since it is a fixed blob path segment."
  }
}
