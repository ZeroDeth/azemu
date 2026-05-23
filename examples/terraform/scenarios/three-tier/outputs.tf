output "resource_group_id" {
  description = "Resource group ARM ID"
  value       = azurerm_resource_group.main.id
}

output "vnet_id" {
  description = "Virtual network ARM ID"
  value       = azurerm_virtual_network.main.id
}

output "web_subnet_id" {
  description = "Web tier subnet ARM ID"
  value       = azurerm_subnet.web.id
}

output "app_subnet_id" {
  description = "App tier subnet ARM ID"
  value       = azurerm_subnet.app.id
}

output "data_subnet_id" {
  description = "Data tier subnet ARM ID"
  value       = azurerm_subnet.data.id
}

output "lb_id" {
  description = "Load balancer ARM ID"
  value       = azurerm_lb.web.id
}

output "lb_public_ip" {
  description = "Load balancer public IP address"
  value       = azurerm_public_ip.lb.ip_address
}

output "appgw_id" {
  description = "Application gateway ARM ID"
  value       = azurerm_application_gateway.main.id
}
