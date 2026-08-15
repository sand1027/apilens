import { Controller, Get } from '@nestjs/common';

@Controller()
export class HealthController {
  @Get('/health')
  check() {
    return { status: 'ok' };
  }
}

// A plain helper class with a coincidentally-named decorated method must
// NOT be treated as a route — it has no @Controller() decorator.
class NotAController {
  @Get('/should-not-count')
  fake() {
    return {};
  }
}
