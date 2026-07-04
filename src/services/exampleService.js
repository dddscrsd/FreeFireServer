// Example service layer. Replace with real implementations.

async function listUsers() {
  // Placeholder: in real code, fetch from DB or other services
  return [
    { id: 1, name: 'Alice' },
    { id: 2, name: 'Bob' }
  ];
}

module.exports = { listUsers };
