export interface Resource {
  id: string;
  name: string;
  type: string;
  location: string;
  tags?: Record<string, string>;
  properties?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface HealthResponse {
  status: string;
  version: string;
  uptime_seconds: number;
  /** "in-memory" or "file-backed". Reported by the emulator, never assumed. */
  store: string;
  /** Key algorithm of the emulator's self-signed serving cert. */
  tls: string;
}

export interface RequestLogEntry {
  ts: string;
  method: 'GET' | 'PUT' | 'POST' | 'DELETE' | 'HEAD';
  path: string;
  status: number;
  durationMs: number;
}

export type CategoryCode =
  | 'RG' | 'VN' | 'SN' | 'NS' | 'PI' | 'LB' | 'AG'
  | 'KV' | 'DN' | 'AK' | 'ST' | 'RC' | 'CD' | 'OT';

export const CATEGORY_COLORS: Record<CategoryCode, string> = {
  RG: '#f0883e',
  VN: '#58a6ff',
  SN: '#58a6ff',
  NS: '#58a6ff',
  PI: '#58a6ff',
  LB: '#58a6ff',
  AG: '#58a6ff',
  KV: '#e3b341',
  DN: '#56d364',
  AK: '#56d364',
  ST: '#a371f7',
  RC: '#db61a2',
  CD: '#f0883e',
  OT: '#8b949e',
};

export const TYPE_TO_CATEGORY: Record<string, { code: CategoryCode; label: string }> = {
  'Microsoft.Resources/resourceGroups': { code: 'RG', label: 'Resource groups' },
  'Microsoft.Network/virtualNetworks': { code: 'VN', label: 'Virtual networks' },
  'Microsoft.Network/virtualNetworks/subnets': { code: 'SN', label: 'Subnets' },
  'Microsoft.Network/networkSecurityGroups': { code: 'NS', label: 'Network security groups' },
  'Microsoft.Network/publicIPAddresses': { code: 'PI', label: 'Public IP addresses' },
  'Microsoft.Network/loadBalancers': { code: 'LB', label: 'Load balancers' },
  'Microsoft.Network/applicationGateways': { code: 'AG', label: 'Application gateways' },
  'Microsoft.KeyVault/vaults': { code: 'KV', label: 'Key vaults' },
  'Microsoft.Network/dnszones': { code: 'DN', label: 'DNS zones' },
  'Microsoft.ContainerService/managedClusters': { code: 'AK', label: 'AKS clusters' },
  'Microsoft.Storage/storageAccounts': { code: 'ST', label: 'Storage accounts' },
  'Microsoft.Cache/redis': { code: 'RC', label: 'Redis caches' },
  'Microsoft.Cdn/profiles': { code: 'CD', label: 'CDN profiles' },
  // Key Vault keys live on the data plane, so their ids and types do not
  // follow the ARM provider naming scheme.
  'keyvault/key': { code: 'KV', label: 'Key vault keys' },
};

export function getCategoryForType(resourceType: string): { code: CategoryCode; color: string } | null {
  const entry = TYPE_TO_CATEGORY[resourceType];
  if (!entry) return null;
  return { code: entry.code, color: CATEGORY_COLORS[entry.code] };
}

export function getResourceGroup(armId: string): string | null {
  const match = armId.match(/\/resourceGroups\/([^/]+)/i);
  return match ? match[1] : null;
}

/**
 * Key Vault data-plane ids look like /keyvault/{vault}/keys/{name}/{version}
 * and carry no /resourceGroups segment, so getResourceGroup alone files them
 * under "unknown". Resolve them through the vault they belong to.
 */
export function resolveResourceGroup(
  resource: Resource,
  all: Resource[],
): string | null {
  const direct = getResourceGroup(resource.id);
  if (direct) return direct;

  const vault = resource.id.match(/^\/keyvault\/([^/]+)/i)?.[1];
  if (!vault) return null;

  const owner = all.find(
    (r) =>
      r.type === 'Microsoft.KeyVault/vaults' &&
      r.name.toLowerCase() === vault.toLowerCase(),
  );
  return owner ? getResourceGroup(owner.id) : null;
}

/**
 * The store keeps a `.../current` alias pointing at the latest version of a
 * Key Vault key. It is a pointer, not a second resource, so counting or
 * listing it would double every key the user created.
 */
export function isAliasResource(resource: Resource): boolean {
  return resource.type.endsWith('/current');
}

export const METHOD_COLORS: Record<string, string> = {
  GET: '#58a6ff',
  PUT: '#56d364',
  POST: '#e3b341',
  DELETE: '#f85149',
  HEAD: '#8b949e',
  PATCH: '#d29922',
};

export function statusColor(code: number): string {
  if (code === 202) return '#d29922';
  if (code >= 200 && code < 300) return '#3fb950';
  return '#f85149';
}
