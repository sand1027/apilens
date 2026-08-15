const express = require('express');
const app = express();

app.get('/health', (req, res) => {
  res.json({ status: 'ok' });
});

app.get('/api/users', (req, res) => {
  res.json({ data: [] });
});

app.post('/api/users', (req, res) => {
  res.status(201).json({ data: req.body });
});

// Dynamic route registration — should be skipped, not guessed.
const dynamicPath = computePath();
app.get(dynamicPath, (req, res) => {
  res.json({});
});

module.exports = app;
