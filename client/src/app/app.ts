import {Component, signal} from '@angular/core';
import {SpanDTO, TracesService} from './api';
import {takeUntilDestroyed} from '@angular/core/rxjs-interop';
import {KeyValuePipe} from '@angular/common';

@Component({
  selector: 'app-root',
  imports: [
    KeyValuePipe
  ],
  templateUrl: './app.html',
  styleUrl: './app.scss'
})
export class App {
  protected readonly title = signal('minitrace-ui');

  readonly spans = signal<SpanDTO[]>([]);
  traces = signal<Map<string, Trace[]>>(new Map());

  constructor(private tracesService: TracesService) {

    this.tracesService.getTraces().pipe(
      takeUntilDestroyed()
    ).subscribe(val => {
      this.spans.set(val)
      console.log(this.spans());
      this.buildTracesWithWidths()
    });
  }


  buildTracesWithWidths(): any {
    const traceMap = new Map<string, Trace[]>();

    for (const rootSpan of this.spans()) {
      traceMap.set(rootSpan.id, this.calculateLengths(rootSpan));
    }

    console.log(traceMap);
    this.traces.set(traceMap)
  }

  calculateLengths(spanDTO: SpanDTO): Trace[] {
    const traces: Trace[] = [];
    const rootDuration = spanDTO.duration;

    traces.push({
      parentId: spanDTO.id,
      name: spanDTO.name,
      lengthInPercent: 100,
      durationInSeconds: Math.round(spanDTO.duration / 1e9 * 1000) / 1000
    })

    function walk(span: SpanDTO): void {
      for (const child of span.children) {
        traces.push({
          parentId: span.id,
          name: child.name,
          lengthInPercent: (child.duration / rootDuration) * 100,
          durationInSeconds: Math.round(child.duration / 1e9 * 1000) / 1000
        });

        walk(child);
      }
    }

    walk(spanDTO);

    return traces;
  }
}
