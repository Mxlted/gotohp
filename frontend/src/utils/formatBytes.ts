const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const scale = Math.max(0, Math.min(units.length - 1, Math.floor(Math.log2(bytes) / 10)))
  const value = Number((bytes / 1024 ** scale).toFixed(scale === 0 ? 0 : 1))
  return `${value} ${units[scale]}`
}
