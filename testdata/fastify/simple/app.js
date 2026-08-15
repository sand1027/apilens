const fastify = require('fastify')();

fastify.get('/health', async (req, reply) => { return { status: 'ok' }; });
fastify.get('/api/users', async (req, reply) => { return { data: [] }; });
fastify.post('/api/users', async (req, reply) => { return { data: req.body }; });

// Dynamic route registration — should be skipped, not guessed.
const dynamicPath = computePath();
fastify.get(dynamicPath, async (req, reply) => { return {}; });

module.exports = fastify;
