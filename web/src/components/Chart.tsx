import React, { useEffect } from 'react';
import { BarChart, LineChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { QueryRow } from '../api';
import { ThemeMode, VisualizationType } from '../types';

echarts.use([BarChart, CanvasRenderer, GridComponent, LegendComponent, LineChart, TooltipComponent]);

const primaryColor = '#2563eb';
const secondaryColor = '#64748b';

export function Chart({ rows, themeMode, type }: { rows: QueryRow[]; themeMode: ThemeMode; type: VisualizationType }) {
  const ref = React.useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!ref.current) return;
    const chart = echarts.init(ref.current, undefined, { renderer: 'canvas' });
    const seriesNames = Array.from(new Set(rows.map((row) => row.series || 'all')));
    const isDark = themeMode === 'dark';
    const colors = [primaryColor, '#0ea5e9', '#22c55e', '#f59e0b', '#ef4444', secondaryColor];
    const textColor = isDark ? '#dbeafe' : '#0f172a';
    const mutedTextColor = isDark ? '#94a3b8' : secondaryColor;
    const borderColor = isDark ? '#334155' : '#cbd5e1';
    const surfaceColor = isDark ? '#0f172a' : '#ffffff';
    const tooltipBg = isDark ? '#111827' : '#ffffff';
    chart.setOption({
      backgroundColor: surfaceColor,
      color: colors,
      grid: { top: 28, right: 28, bottom: 42, left: 58 },
      legend: { top: 0, type: 'scroll', textStyle: { color: mutedTextColor } },
      tooltip: {
        trigger: 'axis',
        backgroundColor: tooltipBg,
        borderColor,
        textStyle: { color: textColor }
      },
      xAxis: {
        type: 'time',
        axisLabel: { color: mutedTextColor },
        axisLine: { lineStyle: { color: borderColor } },
        axisTick: { lineStyle: { color: borderColor } },
        splitLine: { lineStyle: { color: borderColor, opacity: isDark ? 0.35 : 0.55 } }
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: mutedTextColor },
        axisLine: { lineStyle: { color: borderColor } },
        splitLine: { lineStyle: { color: borderColor, opacity: isDark ? 0.35 : 0.55 } }
      },
      series: seriesNames.map((name) => ({
        name,
        type: type === 'bar' ? 'bar' : 'line',
        areaStyle: type === 'area' ? { opacity: isDark ? 0.18 : 0.14 } : undefined,
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
  }, [rows, themeMode, type]);

  return <div className="chart" ref={ref} />;
}
