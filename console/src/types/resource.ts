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
