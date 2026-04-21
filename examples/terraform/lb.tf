resource "azurerm_lb" "example" {
  name                = "${var.prefix}-lb"
  resource_group_name = azurerm_resource_group.example.name
  location            = azurerm_resource_group.example.location
  sku                 = "Standard"

  frontend_ip_configuration {
    name                 = "fe-config"
    public_ip_address_id = azurerm_public_ip.example.id
  }

  tags = {
    environment = "local"
    managed_by  = "azemu"
  }
}

resource "azurerm_lb_backend_address_pool" "example" {
  name            = "${var.prefix}-pool"
  loadbalancer_id = azurerm_lb.example.id
}

resource "azurerm_lb_probe" "example" {
  name                = "${var.prefix}-probe"
  loadbalancer_id     = azurerm_lb.example.id
  protocol            = "Http"
  port                = 80
  request_path        = "/health"
  interval_in_seconds = 15
  number_of_probes    = 2
}

resource "azurerm_lb_rule" "example" {
  name                           = "${var.prefix}-rule"
  loadbalancer_id                = azurerm_lb.example.id
  protocol                       = "Tcp"
  frontend_port                  = 80
  backend_port                   = 80
  frontend_ip_configuration_name = "fe-config"
  backend_address_pool_ids       = [azurerm_lb_backend_address_pool.example.id]
  probe_id                       = azurerm_lb_probe.example.id
}
