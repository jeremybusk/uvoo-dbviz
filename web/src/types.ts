export type VisualizationType = 'line' | 'bar' | 'area';

export type ThemeMode = 'light' | 'dark';

export type QueryState = {
  dataset: string;
  sourceId: string;
  groupBy: string;
  measure: string;
  aggregation: string;
  from: string;
  to: string;
};
