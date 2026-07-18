// DIRECTIVE_SCHEMA describes every dnsmasq directive the config editor knows
// how to render. The schema serves two purposes:
//   1. Decide which control (bool / string / list / structured row) to draw.
//   2. Group directives visually (dns / dhcp / pxe / log / other).
//
// Directives NOT listed here are still editable through the "+ custom
// directive" picker; they just fall back to the generic 'string' control in
// the 'other' group.
export const DIRECTIVE_SCHEMA = {
  // --- DNS / Network ---
  'domain':            { type: 'string', group: 'dns' },
  'no-resolv':         { type: 'bool', group: 'dns' },
  'domain-needed':     { type: 'bool', group: 'dns' },
  'bogus-priv':        { type: 'bool', group: 'dns' },
  'expand-hosts':      { type: 'bool', group: 'dns' },
  'bind-interfaces':   { type: 'bool', group: 'dns' },
  'no-hosts':          { type: 'bool', group: 'dns' },
  'no-poll':           { type: 'bool', group: 'dns' },
  'strict-order':      { type: 'bool', group: 'dns' },
  'all-servers':       { type: 'bool', group: 'dns' },
  'clear-on-reload':   { type: 'bool', group: 'dns' },
  'resolv-file':       { type: 'string', group: 'dns' },
  'listen-address':    { type: 'list', group: 'dns' },
  'except-interface':  { type: 'list', group: 'dns' },
  'interface':         { type: 'list', group: 'dns' },
  // Forwarding rules: server=/domain/upstream and server=upstream.
  'server':            { type: 'forwarding', group: 'dns' },
  // Wildcard/static DNS records managed elsewhere as "Aliases".
  'address':           { type: 'list', group: 'dns' },
  'local':             { type: 'list', group: 'dns' },

  // --- DHCP ---
  'dhcp-range':        { type: 'dhcprange', group: 'dhcp' },
  'dhcp-option':       { type: 'dhcpoption', group: 'dhcp' },
  'dhcp-lease-max':    { type: 'string', group: 'dhcp' },
  'dhcp-authoritative':{ type: 'bool', group: 'dhcp' },
  'dhcp-no-override':  { type: 'bool', group: 'dhcp' },
  'dhcp-hostsfile':    { type: 'string', group: 'dhcp' },
  'dhcp-leasefile':    { type: 'string', group: 'dhcp' },
  'no-dhcp-interface': { type: 'list', group: 'dhcp' },

  // --- PXE / netboot ---
  // dhcp-boot sets the boot file (e.g. "dhcp-boot=pxelinux.0,,next-server-ip").
  // dhcp-match assigns a tag based on a client request attribute.
  // pxe-service maps a PXE service type to a menu entry / boot file.
  // pxe-prompt shows a boot menu prompt with a timeout.
  'dhcp-boot':         { type: 'list', group: 'pxe' },
  'dhcp-match':        { type: 'list', group: 'pxe' },
  'dhcp-mac':          { type: 'list', group: 'pxe' },
  'pxe-service':       { type: 'list', group: 'pxe' },
  'pxe-prompt':        { type: 'string', group: 'pxe' },

  // --- Logging ---
  'log-queries':       { type: 'bool', group: 'log' },
  'log-dhcp':          { type: 'bool', group: 'log' },
  'log-facility':      { type: 'string', group: 'log' },
  'log-async':         { type: 'string', group: 'log' }
}

export const GROUP_ORDER = ['dns', 'dhcp', 'pxe', 'log', 'other']
export const GROUP_LABELS = {
  dns: 'config.groupDns',
  dhcp: 'config.groupDhcp',
  pxe: 'config.groupPxe',
  log: 'config.groupLog',
  other: 'config.groupOther'
}

export function schemaFor(key) {
  return DIRECTIVE_SCHEMA[key] || { type: 'string', group: 'other' }
}

export function defaultDirective(key) {
  const s = schemaFor(key)
  if (s.type === 'bool') return { key, value: '', active: true }
  if (s.type === 'list') return { key, value: '', active: true }
  if (s.type === 'dhcprange') return { key, value: ',,,,', active: true }
  if (s.type === 'dhcpoption') return { key, value: 'option:router,', active: true }
  if (s.type === 'forwarding') return { key, value: '/', active: true }
  return { key, value: '', active: true }
}

// Common DHCP option keys that dnsmasq accepts by name (option:<name>).
// Numbers (3, 6, ...) are also valid but the named form is more readable.
export const DHCP_OPTION_PRESETS = [
  { key: 'option:router',         number: '3',  label: 'Default Gateway',        valueHint: '192.168.1.1',            multi: false },
  { key: 'option:dns-server',     number: '6',  label: 'DNS Servers',            valueHint: '192.168.1.1,8.8.8.8',    multi: true  },
  { key: 'option:domain',         number: '15', label: 'Domain Name',            valueHint: 'lan',                     multi: false },
  { key: 'option:broadcast',      number: '28', label: 'Broadcast Address',      valueHint: '192.168.1.255',           multi: false },
  { key: 'option:ntp-server',     number: '42', label: 'NTP Servers',            valueHint: '192.168.1.1,time.cloudflare.com', multi: true },
  { key: 'option:netbios-ns',     number: '44', label: 'NetBIOS Name Servers',   valueHint: '192.168.1.1',             multi: true  },
  { key: 'option:netbios-scope',  number: '47', label: 'NetBIOS Scope',          valueHint: '',                        multi: false },
  { key: 'option:vendor-class',   number: '43', label: 'Vendor Class Identifier',valueHint: 'MSFT 5.0',                multi: false },
  { key: 'option:tftp-server',    number: '66', label: 'TFTP Server Name',       valueHint: 'pxe.lan',                 multi: false },
  { key: 'option:bootfile-name',  number: '67', label: 'Boot File Name',         valueHint: 'pxelinux.0',              multi: false }
]
