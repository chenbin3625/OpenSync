export type FileSizeUnit = 'B' | 'KB' | 'MB' | 'GB' | 'TB';

export const MAX_SAFE_FILE_SIZE_BYTES = Number.MAX_SAFE_INTEGER;

export const fileSizeUnitOptions = [
  { value: 'B', label: 'B' },
  { value: 'KB', label: 'KB' },
  { value: 'MB', label: 'MB' },
  { value: 'GB', label: 'GB' },
  { value: 'TB', label: 'TB' },
] satisfies Array<{ value: FileSizeUnit; label: FileSizeUnit }>;

const fileSizeUnitFactors: Record<FileSizeUnit, number> = {
  B: 1,
  KB: 1024,
  MB: 1024 ** 2,
  GB: 1024 ** 3,
  TB: 1024 ** 4,
};

export function normalizeFileSizeUnit(unit?: string | null): FileSizeUnit {
  return fileSizeUnitOptions.some((option) => option.value === unit) ? unit as FileSizeUnit : 'MB';
}

export function fileSizeToBytes(value?: number | null, unit?: string | null): number {
  const normalized = Number(value || 0);
  if (!Number.isFinite(normalized) || normalized <= 0) return 0;
  const factor = fileSizeUnitFactors[normalizeFileSizeUnit(unit)];
  const raw = normalized * factor;
  if (!Number.isFinite(raw) || raw >= MAX_SAFE_FILE_SIZE_BYTES) {
    return MAX_SAFE_FILE_SIZE_BYTES;
  }
  return Math.round(raw);
}

export function splitBytesToFileSize(bytes?: number | null): { value: number; unit: FileSizeUnit } {
  const raw = Number(bytes || 0);
  if (!Number.isFinite(raw) || raw <= 0) {
    return { value: 0, unit: 'MB' };
  }

  const normalized = raw >= MAX_SAFE_FILE_SIZE_BYTES
    ? MAX_SAFE_FILE_SIZE_BYTES
    : Math.round(raw);

  // Pick the largest unit that preserves an exact round-trip back to bytes.
  const units = [...fileSizeUnitOptions].map((option) => option.value).reverse();
  for (const candidate of units) {
    const factor = fileSizeUnitFactors[candidate];
    if (normalized < factor) continue;
    const exact = normalized / factor;
    const rounded = Number.isInteger(exact) ? exact : Number(exact.toFixed(2));
    if (Math.round(rounded * factor) === normalized) {
      return { value: rounded, unit: candidate };
    }
  }

  return { value: normalized, unit: 'B' };
}
