export type VisualizationType = 'line' | 'bar' | 'area';

export type ThemeMode = 'light' | 'dark';

export type RelativeRangeUnit = 'minutes' | 'hours' | 'days' | 'weeks' | 'months' | 'years';

export type RelativeRange = {
  value: number;
  unit: RelativeRangeUnit;
};

export type JwtClaims = Record<string, unknown>;

export type QueryState = {
  dataset: string;
  sourceId: string;
  mode?: 'builder' | 'sql';
  sql?: string;
  groupBy: string;
  measure: string;
  aggregation: string;
  from: string;
  to: string;
  search: string;
  limit: number;
};
