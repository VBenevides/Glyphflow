export function sessionDeviceLabel(userAgent?: string, ipAddress?: string): string {
  const agent = userAgent?.trim() ?? ''
  const ip = ipAddress?.trim() ?? ''
  if (!agent) return ip ? `Unknown device · ${ip}` : 'Unknown device'
  let browser = ''
  if (/Edg\/?[\w.]*/i.test(agent)) browser = 'Edge'
  else if (/OPR\/?[\w.]*/i.test(agent)) browser = 'Opera'
  else if (/Chrome\/?[\w.]*/i.test(agent)) browser = 'Chrome'
  else if (/Firefox\/?[\w.]*/i.test(agent)) browser = 'Firefox'
  else if (/Safari\/?[\w.]*/i.test(agent)) browser = 'Safari'
  else if (/MSIE|Trident/i.test(agent)) browser = 'Internet Explorer'
  let device = ''
  if (/Android|iPhone|iPad|iPod|Mobile/i.test(agent)) device = 'Mobile'
  else if (/Windows|Macintosh|Linux|CrOS/i.test(agent)) device = 'Desktop'
  const ipSuffix = ip ? ` · ${ip}` : ''
  if (browser && device) return `${device} · ${browser}${ipSuffix}`
  const agentLabel = agent.length > 160 ? `${agent.slice(0, 160)}…` : agent
  return `${agentLabel}${ipSuffix}`
}
