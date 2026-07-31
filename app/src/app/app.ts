import { DatePipe, JsonPipe } from '@angular/common';
import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { finalize } from 'rxjs';
import { ApiService, TraceDetail, TraceFilters, TraceSpan, TraceSummary } from './api.service';

interface TimelineSpan extends TraceSpan {
  depth: number;
  hasChildren: boolean;
  offsetPercent: number;
  widthPercent: number;
  durationNano: number;
}

@Component({
  selector: 'app-root',
  imports: [DatePipe, FormsModule, JsonPipe],
  templateUrl: './app.html',
  styleUrl: './app.scss'
})
export class App {
  private readonly api = inject(ApiService);
  private readonly destroyRef = inject(DestroyRef);

  readonly traces = signal<TraceSummary[]>([]);
  readonly services = signal<string[]>([]);
  readonly selectedTrace = signal<TraceDetail | null>(null);
  readonly selectedSpan = signal<TraceSpan | null>(null);
  readonly loading = signal(true);
  readonly detailLoading = signal(false);
  readonly error = signal('');
  readonly lastUpdated = signal<Date | null>(null);
  readonly filters = signal<TraceFilters>({ service: '', query: '', errorsOnly: false });
  readonly collapsedSpans = signal<ReadonlySet<string>>(new Set());

  readonly timeline = computed<TimelineSpan[]>(() => {
    const trace = this.selectedTrace();
    if (!trace?.spans.length) return [];

    const start = Math.min(...trace.spans.map(span => Date.parse(span.startTime)));
    const end = Math.max(...trace.spans.map(span => Date.parse(span.endTime)));
    const total = Math.max(end - start, 0.01);
    const byId = new Map(trace.spans.map(span => [span.spanId, span]));
    const parentIds = new Set(trace.spans.map(span => span.parentSpanId).filter(Boolean));
    const collapsed = this.collapsedSpans();

    const depthOf = (span: TraceSpan): number => {
      let depth = 0;
      let parentId = span.parentSpanId;
      const visited = new Set<string>();
      while (parentId && byId.has(parentId) && !visited.has(parentId)) {
        visited.add(parentId);
        depth++;
        parentId = byId.get(parentId)?.parentSpanId;
      }
      return depth;
    };

    const isHidden = (span: TraceSpan): boolean => {
      let parentId = span.parentSpanId;
      const visited = new Set<string>();
      while (parentId && byId.has(parentId) && !visited.has(parentId)) {
        if (collapsed.has(parentId)) return true;
        visited.add(parentId);
        parentId = byId.get(parentId)?.parentSpanId;
      }
      return false;
    };

    return trace.spans.filter(span => !isHidden(span)).map(span => {
      const spanStart = Date.parse(span.startTime);
      const spanEnd = Date.parse(span.endTime);
      return {
        ...span,
        depth: depthOf(span),
        hasChildren: parentIds.has(span.spanId),
        offsetPercent: Math.max(0, ((spanStart - start) / total) * 100),
        widthPercent: Math.max(0.6, ((spanEnd - spanStart) / total) * 100),
        durationNano: Math.max(0, (spanEnd - spanStart) * 1_000_000)
      };
    });
  });

  constructor() {
    this.refresh();
    const timer = window.setInterval(() => this.refresh(false), 10_000);
    this.destroyRef.onDestroy(() => window.clearInterval(timer));
  }

  refresh(showLoader = true): void {
    if (showLoader) this.loading.set(true);
    this.error.set('');
    const filters = this.filters();
    this.api.traces(filters).pipe(
      finalize(() => this.loading.set(false))
    ).subscribe({
      next: traces => {
        this.traces.set(traces);
        this.lastUpdated.set(new Date());
        const selected = this.selectedTrace();
        if (!selected && traces.length) {
          this.openTrace(traces[0]);
        } else if (selected && !traces.some(trace => trace.traceId === selected.traceId)) {
          this.selectedTrace.set(null);
          this.selectedSpan.set(null);
        }
      },
      error: () => this.error.set('The trace server is currently unavailable.')
    });

    this.api.services().subscribe({
      next: services => this.services.set(services)
    });
  }

  updateFilter(patch: Partial<TraceFilters>): void {
    this.filters.update(current => ({ ...current, ...patch }));
    this.refresh();
  }

  openTrace(summary: TraceSummary): void {
    this.detailLoading.set(true);
    this.selectedSpan.set(null);
    this.collapsedSpans.set(new Set());
    this.api.trace(summary.traceId).pipe(
      finalize(() => this.detailLoading.set(false))
    ).subscribe({
      next: trace => this.selectedTrace.set(trace),
      error: () => this.error.set('The trace could not be loaded.')
    });
  }

  selectSpan(span: TraceSpan): void {
    this.selectedSpan.set(span);
  }

  toggleSpan(spanId: string): void {
    this.collapsedSpans.update(current => {
      const next = new Set(current);
      if (next.has(spanId)) {
        next.delete(spanId);
      } else {
        next.add(spanId);
      }
      return next;
    });
  }

  isCollapsed(spanId: string): boolean {
    return this.collapsedSpans().has(spanId);
  }

  formatDuration(nanoseconds: number): string {
    if (nanoseconds < 1_000) return `${Math.round(nanoseconds)} ns`;
    if (nanoseconds < 1_000_000) return `${(nanoseconds / 1_000).toFixed(1)} µs`;
    if (nanoseconds < 1_000_000_000) return `${(nanoseconds / 1_000_000).toFixed(1)} ms`;
    return `${(nanoseconds / 1_000_000_000).toFixed(2)} s`;
  }

  shortId(id: string): string {
    return id.slice(0, 8);
  }

  attributeEntries(span: TraceSpan | null): [string, unknown][] {
    return Object.entries(span?.attributes ?? {});
  }
}
