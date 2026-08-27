export function sessionDeviceLabel(userAgent?: string, ipAddress?: string): string {
  const agent = userAgent?.trim() ?? ''
  const ip = ipAddress?.trim() ?? ''
  if (!agent) return ip ? `Unknown device · ${ip}` : 'Unknown device'
  const browser = /Edg\/?[\w.]*/i.test(agent) ? 'Edge' : /OPR\/?[\w.]*/i.test(agent) ? 'Opera' : /Chrome\/?[\w.]*/i.test(agent) ? 'Chrome' : /Firefox\/?[\w.]*/i.test(agent) ? 'Firefox' : /Safari\/?[\w.]*/i.test(agent) ? 'Safari' : /MSIE|Trident/i.test(agent) ? 'Internet Explorer' : ''
  const device = /Android|iPhone|iPad|iPod|Mobile/i.test(agent) ? 'Mobile' : /Windows|Macintosh|Linux|CrOS/i.test(agent) ? 'Desktop' : ''
  if (browser && device) return `${device} · ${browser}${ip ? ` · ${ip}` : ''}`
  return `${agent.slice(0, 160)}${agent.length > 160 ? '…' : ''}${ip ? ` · ${ip}` : ''}`
}
