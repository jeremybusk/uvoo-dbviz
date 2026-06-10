import React, { useEffect } from 'react';
import { BarChart, LineChart } from 'echarts/charts';
import { DataZoomComponent, GridComponent, LegendComponent, ToolboxComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { QueryRow } from '../api';
import { ThemeMode, VisualizationType } from '../types';

echarts.use([BarChart, CanvasRenderer, DataZoomComponent, GridComponent, LegendComponent, LineChart, ToolboxComponent, TooltipComponent]);

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
      grid: { top: 46, right: 28, bottom: 70, left: 58 },
      legend: { top: 0, left: 0, right: 84, type: 'scroll', textStyle: { color: mutedTextColor } },
      toolbox: {
        top: 0,
        right: 0,
        itemSize: 14,
        iconStyle: { borderColor: mutedTextColor },
        emphasis: { iconStyle: { borderColor: primaryColor } },
        feature: {
          dataZoom: {
            yAxisIndex: 'none',
            title: { zoom: 'Zoom range', back: 'Reset zoom' }
          },
          restore: { title: 'Reset view' }
        }
      },
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
      dataZoom: [
        {
          type: 'inside',
          xAxisIndex: 0,
          filterMode: 'filter',
          zoomOnMouseWheel: 'ctrl',
          moveOnMouseMove: true,
          moveOnMouseWheel: true
        },
        {
          type: 'slider',
          xAxisIndex: 0,
          filterMode: 'filter',
          height: 18,
          bottom: 18,
          borderColor,
          fillerColor: isDark ? 'rgba(37, 99, 235, 0.22)' : 'rgba(37, 99, 235, 0.14)',
          handleStyle: { color: primaryColor },
          moveHandleStyle: { color: primaryColor },
          selectedDataBackground: {
            lineStyle: { color: primaryColor },
            areaStyle: { color: isDark ? 'rgba(37, 99, 235, 0.28)' : 'rgba(37, 99, 235, 0.18)' }
          },
          textStyle: { color: mutedTextColor }
        }
      ],
      series: seriesNames.map((name) => ({
        name,
        type: type === 'bar' ? 'bar' : 'line',
        areaStyle: type === 'area' ? { opacity: isDark ? 0.18 : 0.14 } : undefined,
        showSymbol: false,
        data: rows.filter((row) => (row.series || 'all') === name).map((row) => [row.ts * 1000, row.value])
      }))
    });
    let resizeFrame = 0;
    const resize = () => {
      if (resizeFrame) window.cancelAnimationFrame(resizeFrame);
      resizeFrame = window.requestAnimationFrame(() => chart.resize());
    };
    const resizeObserver = typeof ResizeObserver === 'function'
      ? new ResizeObserver(resize)
      : null;
    if (resizeObserver) resizeObserver.observe(ref.current);
    window.addEventListener('resize', resize);
    resize();
    return () => {
      if (resizeFrame) window.cancelAnimationFrame(resizeFrame);
      resizeObserver?.disconnect();
      window.removeEventListener('resize', resize);
      chart.dispose();
    };
  }, [rows, themeMode, type]);

  return <div className="chart" ref={ref} />;
}
