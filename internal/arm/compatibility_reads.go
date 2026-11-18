package arm

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *Router) getStorageAccountMigration(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "accountName")
	parent := storageAccountID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), account)
	if !a.requireReadParent(w, parent) {
		return
	}

	// The SDK requires 200 even though no migration is tracked by azemu.
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         parent + "/accountMigrations/default",
		"name":       "default",
		"type":       "Microsoft.Storage/storageAccounts/accountMigrations",
		"properties": map[string]any{},
	})
}

func (a *Router) getAKSMaintenanceConfiguration(w http.ResponseWriter, r *http.Request) {
	parent := aksClusterID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "clusterName"))
	if !a.requireReadParent(w, parent) {
		return
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
		fmt.Sprintf("Maintenance configuration %q was not found.", chi.URLParam(r, "configurationName")))
}

func (a *Router) getRedisPatchSchedule(w http.ResponseWriter, r *http.Request) {
	parent := redisCacheID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "cacheName"))
	if !a.requireReadParent(w, parent) {
		return
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "The default Redis patch schedule was not found.")
}

func (a *Router) requireReadParent(w http.ResponseWriter, id string) bool {
	if _, ok := a.store.Get(id); ok {
		return true
	}
	writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", fmt.Sprintf("Parent resource %q was not found.", id))
	return false
}
