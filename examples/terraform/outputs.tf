output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.example.id
}

output "vnet_id" {
  description = "Virtual network ARM ID"
  value       = azurerm_virtual_network.example.id
}

output "subnet_id" {
  description = "Subnet ARM ID"
  value       = azurerm_subnet.example.id
}
