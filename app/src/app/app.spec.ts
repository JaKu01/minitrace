import { TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { ApiService } from './api.service';
import { App } from './app';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [{
        provide: ApiService,
        useValue: {
          traces: () => of([]),
          services: () => of([]),
          trace: () => of(null)
        }
      }]
    }).compileComponents();
  });

  it('creates the tracing dashboard', () => {
    const fixture = TestBed.createComponent(App);
    fixture.detectChanges();
    expect(fixture.componentInstance).toBeTruthy();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('minitrace');
  });
});
