variable "location" {
  description = "Azure region (passed through to azemu, stored but not validated)"
  type        = string
  default     = "uksouth"
}

variable "prefix" {
  description = "Name prefix for all resources"
  type        = string
  default     = "azemu-redis"

  # The prefix is reused for the Redis cache (${prefix}-cache) and the Key
  # Vault (${prefix}-kv). Key Vault names are the tighter constraint (3-24
  # chars, alphanumeric and hyphens, must start with a letter and not end
  # with a hyphen), so validate against that here to fail at plan time rather
  # than at apply time. Capping the prefix at 21 keeps "${prefix}-kv" within
  # the 24-char Key Vault limit.
  validation {
    condition = (
      length(var.prefix) >= 1 &&
      length(var.prefix) <= 21 &&
      can(regex("^[A-Za-z][A-Za-z0-9-]*[A-Za-z0-9]$", var.prefix))
    )
    error_message = "prefix must be 1-21 characters, contain only letters, numbers, and hyphens, start with a letter, and end with a letter or number."
  }
}
