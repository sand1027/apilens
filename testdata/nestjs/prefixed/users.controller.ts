import { Controller, Get, Post, Param } from '@nestjs/common';

@Controller('api/users')
export class UsersController {
  @Get()
  list() {
    return { data: [] };
  }

  @Get(':id')
  getOne(@Param('id') id: string) {
    return { data: {} };
  }

  @Post()
  create() {
    return { data: {} };
  }
}
