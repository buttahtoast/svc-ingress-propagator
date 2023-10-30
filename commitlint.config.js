const Configuration = {
    extends: ['@commitlint/config-conventional'],
    plugins: ['commitlint-plugin-function-rules'],
    rules: {
      'type-enum': [2, 'always', ['chore', 'ci', 'docs', 'feat', 'test', 'fix', 'sec']],
      'body-max-line-length': [1, 'always', 500],
    },
    defaultIgnores: true,
  };
  
  module.exports = Configuration;