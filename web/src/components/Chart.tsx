import React, { useEffect } from 'react';
import { BarChart, LineChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { QueryRow } from '../api';
import { VisualizationType } from '../types';

echarts.use([BarChart, CanvasRenderer, GridComponent, LegendComponent, LineChart, TooltipComponent]);

export function Chart({ rows, type }: { rows: QueryRow[]; type: VisualizationType }) {
  const ref = React.useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current, undefined, { renderer: 'canvas' });
    const seriesNames = Array.from(new Set(rows.map((row) => row.series || 'all')));
    chart.setOption({
      grid: { top: 28, right: 28, bottom: 42, left: 58 },
      legend: { top: 0, type: 'scroll' },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'time' },
      yAxis: { type: 'value' },
      series: seriesNames.map((name) => ({
        name,
        type: type === 'bar' ? 'bar' : 'line',
        areaStyle: type === 'area' ? {} : undefined,
        showSymbol: false,
        data: rows.filter((row) => (row.series || 'all') === name).map((row) => [row.ts * 1000, row.value])
      }))
    });
    const resize = () => chart.resize();
    window.addEventListener('resize', resize);
    return () => {
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [rows, type]);

  return <div className="chart" ref={ref} />;
}
