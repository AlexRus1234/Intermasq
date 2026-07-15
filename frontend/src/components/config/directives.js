export const DIRECTIVE_SCHEMA = {
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
  'server':            { type: 'list', group: 'dns' },
  'address':           { type: 'list', group: 'dns' },
  'local':             { type: 'list', group: 'dns' },

  'dhcp-range':        { type: 'dhcprange', group: 'dhcp' },
  'dhcp-option':       { type: 'list', group: 'dhcp' },
  'dhcp-lease-max':    { type: 'string', group: 'dhcp' },
  'dhcp-authoritative':{ type: 'bool', group: 'dhcp' },
  'dhcp-no-override':  { type: 'bool', group: 'dhcp' },
  'dhcp-hostsfile':    { type: 'string', group: 'dhcp' },
  'dhcp-leasefile':    { type: 'string', group: 'dhcp' },
  'no-dhcp-interface': { type: 'list', group: 'dhcp' },

  'log-queries':       { type: 'bool', group: 'log' },
  'log-dhcp':          { type: 'bool', group: 'log' },
  'log-facility':      { type: 'string', group: 'log' },
  'log-async':         { type: 'string', group: 'log' }
}

export const GROUP_ORDER = ['dns', 'dhcp', 'log', 'other']
export const GROUP_LABELS = {
  dns: 'config.groupDns',
  dhcp: 'config.groupDhcp',
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
  return { key, value: '', active: true }
}
