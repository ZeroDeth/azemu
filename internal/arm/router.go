package arm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
	"github.com/zerodeth/azemu/internal/store"
)

type Router struct {
	store           store.Store
	azuriteEndpoint string // e.g. "http://azurite:10000", blob service base URL
	kvEndpoint      string // e.g. "https://localhost:4566", Key Vault data-plane base URL
	redisEndpoint   string // e.g. "redis://azemu-redis:6379", Redis sidecar URL
	tokenValidator  TokenValidator
}

// TokenValidator is satisfied by auth.TokenService. Keeping the interface in
// arm avoids coupling ARM handlers directly to the auth package.
type TokenValidator interface {
	ValidateBearerToken(raw, expectedAud string) bool
}

func NewRouter(s store.Store, azuriteEndpoint, kvEndpoint, redisEndpoint string, validators ...TokenValidator) *Router {
	var tokenValidator TokenValidator
	if len(validators) > 0 {
		tokenValidator = validators[0]
	}
	return &Router{
		store:           s,
		azuriteEndpoint: azuriteEndpoint,
		kvEndpoint:      kvEndpoint,
		redisEndpoint:   redisEndpoint,
		tokenValidator:  tokenValidator,
	}
}

func (a *Router) Routes(r chi.Router) {
	// Subscription info
	r.Get("/", a.listSubscriptions)
	r.Get("/{subscriptionID}", a.getSubscription)

	// Provider registration (always succeeds)
	r.Get("/{subscriptionID}/providers", a.listProviders)
	r.Get("/{subscriptionID}/providers/{namespace}", a.getProvider)
	r.Post("/{subscriptionID}/providers/{namespace}/register", a.registerProvider)

	// Resource groups
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.putResourceGroup)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.getResourceGroup)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.deleteResourceGroup)
	r.Get("/{subscriptionID}/resourcegroups", a.listResourceGroups)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.headResourceGroup)
	// List child resources within an RG. The azurerm provider calls this
	// during `terraform destroy` to verify the RG is empty (or to enumerate
	// what would be cascade-deleted) BEFORE issuing the DELETE on the RG.
	// Returning 501 here surfaces as the cryptic provider-side error
	// "a polling status of `Failed` should be surfaced as a PollingFailedError".
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/resources", a.listResourceGroupResources)
	// Subscription-wide resource listing with optional
	// $filter=resourceType eq '...'. The azurerm provider uses it to map a
	// Key Vault vaultUri back to the vault's ARM resource ID.
	r.Get("/{subscriptionID}/resources", a.listSubscriptionResources)

	// Async operation result polling. Every 202 Accepted DELETE sets a
	// Location header pointing here; azurerm's poller GETs it until it sees a
	// terminal status. azemu deletes synchronously, so this always reports
	// Succeeded. Without it, destroy hangs until the provider's delete timeout.
	r.Get("/{subscriptionID}/operationresults/{operationID}", a.getOperationResult)

	// Virtual networks (Microsoft.Network/virtualNetworks)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.putVNet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.getVNet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.headVNet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.deleteVNet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks", a.listVNetsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/virtualnetworks", a.listVNetsBySub)

	// Subnets (Microsoft.Network/virtualNetworks/subnets)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.putSubnet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.getSubnet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.headSubnet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.deleteSubnet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets", a.listSubnets)

	// Network Security Groups (Microsoft.Network/networkSecurityGroups)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.putNSG)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.getNSG)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.headNSG)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.deleteNSG)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups", a.listNSGsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/networksecuritygroups", a.listNSGsBySub)

	// Security Rules (Microsoft.Network/networkSecurityGroups/securityRules)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.putRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.getRule)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.headRule)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.deleteRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules", a.listRules)

	// Public IP addresses (Microsoft.Network/publicIPAddresses)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.putPublicIP)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.getPublicIP)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.headPublicIP)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.deletePublicIP)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses", a.listPublicIPsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/publicipaddresses", a.listPublicIPsBySub)

	// Load Balancers (Microsoft.Network/loadBalancers)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.putLB)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.getLB)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.headLB)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.deleteLB)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers", a.listLBsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/loadbalancers", a.listLBsBySub)

	// Backend Address Pools (Microsoft.Network/loadBalancers/backendAddressPools)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.putLBBackendPool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.getLBBackendPool)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.headLBBackendPool)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.deleteLBBackendPool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools", a.listLBBackendPools)

	// Load Balancing Rules (Microsoft.Network/loadBalancers/loadBalancingRules)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.putLBRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.getLBRule)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.headLBRule)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.deleteLBRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules", a.listLBRules)

	// Probes (Microsoft.Network/loadBalancers/probes)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.putLBProbe)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.getLBProbe)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.headLBProbe)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.deleteLBProbe)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes", a.listLBProbes)

	// Application Gateways (Microsoft.Network/applicationGateways)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.putAppGW)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.getAppGW)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.headAppGW)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.deleteAppGW)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways", a.listAppGWsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/applicationgateways", a.listAppGWsBySub)

	// DNS Zones (Microsoft.Network/dnsZones)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.putDNSZone)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.getDNSZone)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.headDNSZone)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.deleteDNSZone)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones", a.listDNSZonesByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/dnszones", a.listDNSZonesBySub)

	// DNS Record Sets (Microsoft.Network/dnsZones/{recordType}/{recordName})
	// The {recordType} parameter captures the lowercase record type (a, aaaa, cname, txt, mx, srv, ns, soa)
	// after NormalizePath lowercases the path. Handlers uppercase it for storage.
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.putDNSRecordSet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.getDNSRecordSet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.headDNSRecordSet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.deleteDNSRecordSet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}", a.listDNSRecordSetsByType)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/recordsets", a.listAllDNSRecordSets)

	// Storage Accounts (Microsoft.Storage/storageAccounts)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.putStorageAccount)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.getStorageAccount)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.headStorageAccount)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.deleteStorageAccount)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts", a.listStorageAccountsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.storage/storageaccounts", a.listStorageAccountsBySub)
	// listKeys — called by the azurerm provider to populate account key in state.
	// Returns Azurite's well-known key so SDK clients can authenticate against
	// the Azurite sidecar without extra configuration.
	r.Post("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/listkeys", a.listStorageAccountKeys)

	// Azure Cache for Redis (Microsoft.Cache/Redis)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis/{cacheName}", a.putRedisCache)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis/{cacheName}", a.getRedisCache)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis/{cacheName}", a.headRedisCache)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis/{cacheName}", a.deleteRedisCache)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis", a.listRedisCachesByRG)
	r.Get("/{subscriptionID}/providers/microsoft.cache/redis", a.listRedisCachesBySub)
	// listKeys returns deterministic dev keys whose primary value matches the
	// Redis sidecar's --requirepass so SDK clients authenticate against the
	// real Redis data plane without further configuration.
	r.Post("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cache/redis/{cacheName}/listkeys", a.listRedisCacheKeys)

	// Key Vaults (Microsoft.KeyVault/vaults)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.putKeyVault)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.getKeyVault)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.headKeyVault)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.deleteKeyVault)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults", a.listKeyVaultsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.keyvault/vaults", a.listKeyVaultsBySub)
	// azurerm v4 checks for soft-deleted Key Vaults before creating a new one.
	// azemu does not implement soft-delete; always return 404 (no deleted vault found).
	r.Get("/{subscriptionID}/providers/microsoft.keyvault/locations/{location}/deletedvaults/{vaultName}", a.getDeletedKeyVault)
	// azurerm v4 purges the deleted vault after deleting it (purge_protection_enabled=false).
	// azemu does not implement soft-delete/purge; return 200 OK (purge complete).
	// The provider's vaults.VaultsClient#PurgeDeleted sends POST .../purge;
	// the bare DELETE form is kept for raw clients.
	r.Post("/{subscriptionID}/providers/microsoft.keyvault/locations/{location}/deletedvaults/{vaultName}/purge", a.purgeDeletedKeyVault)
	r.Delete("/{subscriptionID}/providers/microsoft.keyvault/locations/{location}/deletedvaults/{vaultName}", a.purgeDeletedKeyVault)

	// Storage file service stub — azurerm v4 polls this endpoint after creating
	// a storage account to wait for the file service to become available.
	// azemu does not implement the file service; return a minimal Succeeded response.
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/fileservices/default", a.getStorageFileService)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/fileservices", a.getStorageFileService)

	// Storage Blob Service properties — azurerm v4 reads this endpoint after
	// creating a storage account to check/encode blob service properties, and
	// writes it when the `blob_properties` block is set (a missing PUT route
	// surfaces as "unexpected status 405" during apply).
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default", a.getBlobServiceProperties)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default", a.putBlobServiceProperties)

	// Storage Blob Containers (Microsoft.Storage/storageAccounts/blobServices/containers)
	// The path segment "default" is a fixed literal (not a parameter) matching the real ARM API.
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.putStorageContainer)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.getStorageContainer)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.headStorageContainer)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.deleteStorageContainer)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers", a.listStorageContainers)

	// CDN Profiles (Microsoft.Cdn/profiles)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}", a.putCDNProfile)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}", a.getCDNProfile)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}", a.headCDNProfile)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}", a.deleteCDNProfile)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles", a.listCDNProfilesByRG)
	r.Get("/{subscriptionID}/providers/microsoft.cdn/profiles", a.listCDNProfilesBySub)

	// CDN Endpoints (Microsoft.Cdn/profiles/endpoints)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}/endpoints/{endpointName}", a.putCDNEndpoint)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}/endpoints/{endpointName}", a.getCDNEndpoint)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}/endpoints/{endpointName}", a.headCDNEndpoint)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}/endpoints/{endpointName}", a.deleteCDNEndpoint)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.cdn/profiles/{profileName}/endpoints", a.listCDNEndpoints)

	// User Assigned Identities (Microsoft.ManagedIdentity/userAssignedIdentities)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}/federatedidentitycredentials/{credentialName}", a.putFederatedIdentityCredential)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}/federatedidentitycredentials/{credentialName}", a.getFederatedIdentityCredential)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}/federatedidentitycredentials/{credentialName}", a.headFederatedIdentityCredential)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}/federatedidentitycredentials/{credentialName}", a.deleteFederatedIdentityCredential)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}/federatedidentitycredentials", a.listFederatedIdentityCredentials)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}", a.putUserAssignedIdentity)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}", a.getUserAssignedIdentity)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}", a.headUserAssignedIdentity)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities/{identityName}", a.deleteUserAssignedIdentity)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.managedidentity/userassignedidentities", a.listUserAssignedIdentitiesByRG)
	r.Get("/{subscriptionID}/providers/microsoft.managedidentity/userassignedidentities", a.listUserAssignedIdentitiesBySub)

	// AKS Clusters (Microsoft.ContainerService/managedClusters)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}", a.putAKSCluster)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}", a.getAKSCluster)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}", a.headAKSCluster)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}", a.deleteAKSCluster)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters", a.listAKSClustersByRG)
	r.Get("/{subscriptionID}/providers/microsoft.containerservice/managedclusters", a.listAKSClustersBySub)
	// Credential listing. azurerm POSTs these on every cluster read to
	// populate kube_config / kube_admin_config; unhandled they 501.
	r.Post("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/listclusterusercredential", a.listAKSClusterUserCredential)
	r.Post("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/listclusteradmincredential", a.listAKSClusterAdminCredential)

	// AKS Agent Pools (Microsoft.ContainerService/managedClusters/agentPools)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/agentpools/{poolName}", a.putAKSNodePool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/agentpools/{poolName}", a.getAKSNodePool)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/agentpools/{poolName}", a.headAKSNodePool)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/agentpools/{poolName}", a.deleteAKSNodePool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.containerservice/managedclusters/{clusterName}/agentpools", a.listAKSNodePools)
}

// KeyVaultDataPlaneRoutes mounts the Key Vault secrets data-plane API.
// Routes are registered under a "/keyvault" prefix in main.go so the full
// paths are "/keyvault/{vaultName}/secrets/{secretName}" etc. The azurerm
// provider discovers these URLs from the vaultUri field returned by the
// management-plane GET/PUT for azurerm_key_vault.
// The "versions" literal is registered before "/{version}" so chi's radix
// trie matches it as a literal segment before the wildcard.
func (a *Router) KeyVaultDataPlaneRoutes(r chi.Router) {
	// keyvault.BaseClient#GetSecret (autorest SDK, used by azurerm v4) constructs
	// the URL as /secrets/{name}/{version}. When secretVersion is empty the
	// resulting path has a trailing slash: /secrets/{name}/. chi v5 does not
	// strip trailing slashes by default, so without StripSlashes the route would
	// fall through to the NotFound handler.
	r.Use(chimw.StripSlashes)

	r.Get("/{vaultName}/secrets/{secretName}/versions", a.listKeyVaultSecretVersions)
	r.Get("/{vaultName}/secrets/{secretName}/{version}", a.getKeyVaultSecretVersion)
	r.Put("/{vaultName}/secrets/{secretName}", a.putKeyVaultSecret)
	r.Get("/{vaultName}/secrets/{secretName}", a.getKeyVaultSecret)
	r.Delete("/{vaultName}/secrets/{secretName}", a.deleteKeyVaultSecret)
	r.Get("/{vaultName}/secrets", a.listKeyVaultSecrets)

	// The azurerm provider polls GET {vaultUri} as a data-plane availability
	// check before nested-item operations; a 404 makes it retry for minutes.
	// Return 200 with an empty object to signal the vault is reachable.
	r.Get("/{vaultName}", a.getKeyVaultDataPlaneRoot)

	// Keys data plane (sign-only RSA scope). Literal segments ("create",
	// "versions") register before "/{version}" wildcards, matching the
	// secrets pattern above. PATCH is registered on both the versionless and
	// versioned paths because autorest's UpdateKey sends /keys/{name}/ when
	// the version is empty (StripSlashes reduces it to /keys/{name}).
	r.Post("/{vaultName}/keys/{keyName}/create", a.createKeyVaultKey)
	// Versionless sign resolves the current pointer; Azure SDKs and az cli
	// call sign without a version to mean "latest".
	r.Post("/{vaultName}/keys/{keyName}/sign", a.signKeyVaultKey)
	r.Post("/{vaultName}/keys/{keyName}/{version}/sign", a.signKeyVaultKey)
	r.Get("/{vaultName}/keys/{keyName}/versions", a.listKeyVaultKeyVersions)
	r.Get("/{vaultName}/keys/{keyName}/{version}", a.getKeyVaultKeyVersion)
	r.Patch("/{vaultName}/keys/{keyName}/{version}", a.updateKeyVaultKey)
	r.Put("/{vaultName}/keys/{keyName}", a.importKeyVaultKey)
	r.Get("/{vaultName}/keys/{keyName}", a.getKeyVaultKey)
	r.Patch("/{vaultName}/keys/{keyName}", a.updateKeyVaultKey)
	r.Delete("/{vaultName}/keys/{keyName}", a.deleteKeyVaultKey)
	r.Get("/{vaultName}/keys", a.listKeyVaultKeys)

	// azurerm v4 fetches certificate contacts on every plan/refresh to detect
	// drift. azemu does not manage certificates; return an empty contact list.
	r.Get("/{vaultName}/certificates/contacts", a.getKeyVaultCertificateContacts)
}

// KeyVaultNestedItemRoutes mounts root-level /keys and /secrets routes.
// The azurerm provider derives data-plane URLs in two ways, neither of which
// carries a vault path segment:
//
//   - vaultUri + /keys/... or /secrets/... for create/set operations, where
//     vaultUri is https://{vault}.vault.localhost[:port]/ (host identifies
//     the vault);
//   - nested-item IDs (kid, secret id) parsed with ParseNestedItemID, which
//     requires the URL path to be exactly /keys/{name}[/{version}], with
//     follow-up requests against {scheme}://{host}/keys/...
//
// These routes resolve the owning vault from the Host header first and fall
// back to an item-name scan (item names are unique enough within a local
// emulator), then inject the vaultName URL param so the same handlers used
// by the path-style routes under /keyvault/{vaultName}/ work unchanged.
func (a *Router) KeyVaultNestedItemRoutes(r chi.Router) {
	// Availability poll: the provider GETs vaultUri root before nested-item
	// operations and retries on 404, so the per-vault host must answer 200.
	r.Get("/", a.getKeyVaultHostRoot)

	r.Route("/keys", func(r chi.Router) {
		r.Use(chimw.StripSlashes)
		r.Post("/{keyName}/create", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.createKeyVaultKey))
		r.Get("/{keyName}/versions", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.listKeyVaultKeyVersions))
		r.Post("/{keyName}/sign", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.signKeyVaultKey))
		r.Post("/{keyName}/{version}/sign", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.signKeyVaultKey))
		r.Get("/{keyName}/{version}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.getKeyVaultKeyVersion))
		r.Patch("/{keyName}/{version}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.updateKeyVaultKey))
		r.Put("/{keyName}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.importKeyVaultKey))
		r.Get("/{keyName}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.getKeyVaultKey))
		r.Patch("/{keyName}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.updateKeyVaultKey))
		r.Delete("/{keyName}", a.withVaultFor("keyName", "keyvault/key/current", "KeyNotFound", a.deleteKeyVaultKey))
	})
	r.Route("/secrets", func(r chi.Router) {
		r.Use(chimw.StripSlashes)
		r.Get("/{secretName}/versions", a.withVaultFor("secretName", "keyvault/secret/current", "SecretNotFound", a.listKeyVaultSecretVersions))
		r.Get("/{secretName}/{version}", a.withVaultFor("secretName", "keyvault/secret/current", "SecretNotFound", a.getKeyVaultSecretVersion))
		r.Put("/{secretName}", a.withVaultFor("secretName", "keyvault/secret/current", "SecretNotFound", a.putKeyVaultSecret))
		r.Get("/{secretName}", a.withVaultFor("secretName", "keyvault/secret/current", "SecretNotFound", a.getKeyVaultSecret))
		r.Delete("/{secretName}", a.withVaultFor("secretName", "keyvault/secret/current", "SecretNotFound", a.deleteKeyVaultSecret))
	})
	// azurerm v4 fetches certificate contacts on every plan/refresh; mirror
	// the path-style stub for the host-based form.
	r.Get("/certificates/contacts", a.getKeyVaultCertificateContacts)

	// Soft-delete purge stubs. With the default features {} block the
	// provider purges deleted keys/secrets after destroy (DELETE
	// /deleted{keys,secrets}/{name}, expecting 204) and may poll the GET
	// form. azemu deletes immediately and keeps no soft-deleted state, so
	// purge is a no-op success and the GET reports nothing to recover.
	r.Delete("/deletedkeys/{keyName}", a.purgeDeletedKeyVaultItem)
	r.Get("/deletedkeys/{keyName}", a.getDeletedKeyVaultItem)
	r.Delete("/deletedsecrets/{secretName}", a.purgeDeletedKeyVaultItem)
	r.Get("/deletedsecrets/{secretName}", a.getDeletedKeyVaultItem)
}

// purgeDeletedKeyVaultItem acknowledges a purge of a soft-deleted key or
// secret. azemu has no soft-delete state, so there is nothing to remove.
func (a *Router) purgeDeletedKeyVaultItem(w http.ResponseWriter, r *http.Request) {
	log.Info().Str("path", r.URL.Path).Msg("key vault deleted-item purge (no-op)")
	w.WriteHeader(http.StatusNoContent)
}

// getDeletedKeyVaultItem reports that no soft-deleted item exists, which
// the provider treats as "already purged".
func (a *Router) getDeletedKeyVaultItem(w http.ResponseWriter, r *http.Request) {
	writeAzureError(w, http.StatusNotFound, "KeyNotFound",
		"Deleted item not found: azemu does not retain soft-deleted Key Vault items.")
}

// vaultNameFromHost extracts the vault name from a per-vault data-plane
// host of the form {vault}.vault.{suffix}, mirroring how the azurerm
// provider treats {vault}.vault.azure.net. Returns false for any other
// host shape (e.g. plain localhost).
func vaultNameFromHost(host string) (string, bool) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	parts := strings.Split(hostname, ".")
	if len(parts) >= 3 && parts[1] == "vault" && parts[0] != "" {
		return parts[0], true
	}
	return "", false
}

// withVaultFor resolves the vault for a root-level data-plane request and
// injects it as the vaultName URL param so the path-style handlers work
// unchanged. Resolution order: Host header ({vault}.vault.*), then a scan
// for an existing item with the requested name. Responds 404 with the given
// error code when neither resolves.
func (a *Router) withVaultFor(paramName, currentType, notFoundCode string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vaultName, ok := vaultNameFromHost(r.Host)
		if !ok {
			itemName := chi.URLParam(r, paramName)
			vaultName, ok = a.findVaultForItem(currentType, itemName)
			if !ok {
				writeAzureError(w, http.StatusNotFound, notFoundCode,
					fmt.Sprintf("A key vault item with name %q was not found in any vault.", itemName))
				return
			}
		}
		chi.RouteContext(r.Context()).URLParams.Add("vaultName", vaultName)
		next(w, r)
	}
}

// getKeyVaultHostRoot answers GET / for per-vault hosts
// ({vault}.vault.localhost). 200 when the vault exists, 404 otherwise.
// Non-vault hosts get a plain 404 so the route stays invisible to other
// root traffic.
func (a *Router) getKeyVaultHostRoot(w http.ResponseWriter, r *http.Request) {
	name, ok := vaultNameFromHost(r.Host)
	if !ok || !a.vaultExists(name) {
		writeAzureError(w, http.StatusNotFound, "VaultNotFound",
			fmt.Sprintf("Vault was not found for host %q.", r.Host))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// findVaultForItem scans the data-plane store for a current-pointer entry of
// the given type and name, returning the owning vault name. Store IDs have
// the shape /keyvault/{vault}/{keys|secrets}/{name}/current.
func (a *Router) findVaultForItem(currentType, itemName string) (string, bool) {
	for _, res := range a.store.List("/keyvault/") {
		if res.Type != currentType || res.Name != itemName {
			continue
		}
		parts := strings.Split(res.ID, "/")
		if len(parts) >= 3 {
			return parts[2], true
		}
	}
	return "", false
}

// --- Subscriptions ---

func (a *Router) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"value": []map[string]interface{}{
			{
				"id":             "/subscriptions/00000000-0000-0000-0000-000000000000",
				"subscriptionId": "00000000-0000-0000-0000-000000000000",
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"displayName":    "azemu-default",
				"state":          "Enabled",
			},
		},
	})
}

func (a *Router) getSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             "/subscriptions/" + subID,
		"subscriptionId": subID,
		"tenantId":       "00000000-0000-0000-0000-000000000001",
		"displayName":    "azemu-default",
		"state":          "Enabled",
	})
}

// --- Providers (always registered) ---

var defaultProviders = []string{
	"Microsoft.Resources", "Microsoft.Network", "Microsoft.Storage",
	"Microsoft.Compute", "Microsoft.KeyVault", "Microsoft.Web",
	"Microsoft.ContainerRegistry", "Microsoft.Dns",
	"Microsoft.ManagedIdentity", "Microsoft.ContainerService",
	"Microsoft.Cache",
}

func (a *Router) listProviders(w http.ResponseWriter, r *http.Request) {
	var providers []map[string]interface{}
	for _, ns := range defaultProviders {
		providers = append(providers, providerEntry(ns))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": providers})
}

func (a *Router) getProvider(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	writeJSON(w, http.StatusOK, providerEntry(ns))
}

func (a *Router) registerProvider(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	writeJSON(w, http.StatusOK, providerEntry(ns))
}

func providerEntry(ns string) map[string]interface{} {
	return map[string]interface{}{
		"id":                "/subscriptions/00000000-0000-0000-0000-000000000000/providers/" + ns,
		"namespace":         ns,
		"registrationState": "Registered",
		"resourceTypes":     []interface{}{},
	}
}

// --- Resource Groups ---

func (a *Router) putResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")

	var body struct {
		Location   string                 `json:"location"`
		Tags       map[string]string      `json:"tags"`
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	// Mirror the validation pattern used by vnet.go / subnet.go: a PUT with
	// no location is rejected with 400 InvalidRequestContent. Previously
	// putResourceGroup predated this pattern and accepted `{}` silently.
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)
	res := &store.Resource{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.Resources/resourceGroups",
		Location: strings.ToLower(body.Location),
		Tags:     normaliseTags(body.Tags),
		Properties: map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put resource group %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("resource group upsert")
	writeJSON(w, status, resourceGroupResponse(res))
}

func (a *Router) getResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceGroupNotFound",
			fmt.Sprintf("Resource group '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, resourceGroupResponse(res))
}

func (a *Router) headResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceGroupNotFound",
			fmt.Sprintf("Resource group '%s' could not be found.", name))
		return
	}

	// ARM returns 202 Accepted with a tracking header for async delete
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listResourceGroups(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	resources := a.store.List(prefix)

	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type == "Microsoft.Resources/resourceGroups" {
			items = append(items, resourceGroupResponse(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// listResourceGroupResources returns every resource the store holds whose
// id is a path-extension of the given resource group, EXCLUDING the
// resource group itself. The azurerm provider calls this during destroy
// to verify the cascade is safe; returning an empty value array for an
// RG that contains nothing is the right behaviour. Query params like
// $expand and $top are accepted but ignored — azemu always returns the
// full result set.
func (a *Router) listResourceGroupResources(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/", subID, rgName)
	resources := a.store.List(prefix)

	items := []map[string]interface{}{}
	for _, res := range resources {
		// Exclude the parent RG itself; the prefix matches it as a
		// substring of any child id.
		if res.Type == "Microsoft.Resources/resourceGroups" {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":         res.ID,
			"name":       res.Name,
			"type":       res.Type,
			"location":   res.Location,
			"tags":       res.Tags,
			"properties": res.Properties,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// listSubscriptionResources implements the subscription-wide Resources List
// API (GET /subscriptions/{sub}/resources). The azurerm provider uses it
// with $filter=resourceType eq 'Microsoft.KeyVault/vaults' to populate its
// Key Vaults cache when mapping a vaultUri back to the vault ARM ID. Only
// the resourceType equality filter is honoured; other filters are ignored
// and the full set is returned. Pagination is not implemented (no nextLink),
// matching the store's bounded size.
func (a *Router) listSubscriptionResources(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)

	wantType := ""
	if f := r.URL.Query().Get("$filter"); f != "" {
		if m := resourceTypeFilterRe.FindStringSubmatch(f); m != nil {
			wantType = m[1]
		}
	}

	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type == "Microsoft.Resources/resourceGroups" {
			continue
		}
		if wantType != "" && !strings.EqualFold(res.Type, wantType) {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":       res.ID,
			"name":     res.Name,
			"type":     res.Type,
			"location": res.Location,
			"tags":     res.Tags,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// resourceTypeFilterRe extracts the type from an OData filter of the form
// "resourceType eq 'Microsoft.KeyVault/vaults'" (case-insensitive key).
var resourceTypeFilterRe = regexp.MustCompile(`(?i)resourcetype\s+eq\s+'([^']+)'`)

func resourceGroupResponse(r *store.Resource) map[string]interface{} {
	return map[string]interface{}{
		"id":       r.ID,
		"name":     r.Name,
		"type":     r.Type,
		"location": r.Location,
		"tags":     r.Tags,
		"properties": map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	}
}

// --- Helpers ---

// normaliseTags ensures tags is always a non-nil map so JSON responses
// serialise as {} instead of null, matching real Azure behaviour.
func normaliseTags(tags map[string]string) map[string]string {
	if tags == nil {
		return map[string]string{}
	}
	return tags
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Error().Err(err).Msg("writeJSON: encode failed")
		http.Error(w, `{"error":{"code":"InternalServerError","message":"response encoding failed"}}`,
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func writeAzureError(w http.ResponseWriter, status int, code, message string) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}); err != nil {
		log.Error().Err(err).Msg("writeAzureError: encode failed")
		http.Error(w, `{"error":{"code":"InternalServerError","message":"error encoding failed"}}`,
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
