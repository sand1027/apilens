const app = require('fastify')();

app.route({
  method: 'GET',
  url: '/api/users/:id',
  handler: async (req, reply) => { return { data: {} }; },
});

app.get('/health', async (req, reply) => { return { status: 'ok' }; });

module.exports = app;
