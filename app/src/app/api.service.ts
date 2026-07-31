import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';

export interface TraceSummary {
  traceId: string;
  name: string;
  serviceName: string;
  startTime: string;
  durationNano: number;
  spanCount: number;
  status: 'ok' | 'error';
}

export interface TraceSpan {
  traceId: string;
  spanId: string;
  parentSpanId?: string;
  serviceName: string;
  name: string;
  startTime: string;
  endTime: string;
  status: 'ok' | 'error';
  attributes?: Record<string, unknown>;
  events?: TraceEvent[];
}

export interface TraceEvent {
  name: string;
  time: string;
  attributes?: Record<string, unknown>;
}

export interface TraceDetail {
  traceId: string;
  durationNano: number;
  spans: TraceSpan[];
}

export interface TraceFilters {
  service: string;
  query: string;
  errorsOnly: boolean;
}

@Injectable({ providedIn: 'root' })
export class ApiService {
  constructor(private readonly http: HttpClient) {}

  traces(filters: TraceFilters): Observable<TraceSummary[]> {
    let params = new HttpParams().set('limit', 200);
    if (filters.service) params = params.set('service', filters.service);
    if (filters.query) params = params.set('q', filters.query);
    if (filters.errorsOnly) params = params.set('status', 'error');
    return this.http.get<TraceSummary[]>('/api/v1/traces', { params });
  }

  trace(traceId: string): Observable<TraceDetail> {
    return this.http.get<TraceDetail>(`/api/v1/traces/${encodeURIComponent(traceId)}`);
  }

  services(): Observable<string[]> {
    return this.http.get<string[]>('/api/v1/services');
  }
}
