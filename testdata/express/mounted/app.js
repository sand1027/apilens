const express = require('express');
const app = express();

const userRouter = express.Router();
userRouter.get('/', (req, res) => res.json({ data: [] }));
userRouter.get('/:id', (req, res) => res.json({ data: {} }));
userRouter.post('/', (req, res) => res.status(201).json({}));

app.use('/api/users', userRouter);

app.get('/health', (req, res) => res.json({ status: 'ok' }));

module.exports = app;
