const { ProxyAgent } = require('undici');
const OpenAI = require('openai');

async function testProxy() {
    console.log('Testing proxy connection...');
    const proxyUrl = process.env.PROXY;

    if (!proxyUrl) {
        console.error('ERROR: PROXY env variable is not set');
        return;
    }

    console.log('Proxy URL:', proxyUrl);

    try {
        const dispatcher = new ProxyAgent(proxyUrl);

        // 1. Test basic connectivity via proxy
        console.log('\n1. Testing basic network via proxy (httpbin.org)...');
        try {
            // Using native fetch with undici dispatcher
            const response = await fetch('https://httpbin.org/ip', { dispatcher });
            const data = await response.json();
            console.log('Success! IP returned:', data);
        } catch (e) {
            console.error('Basic connectivity failed:', e.message);
        }

        // 2. Test OpenAI connection
        console.log('\n2. Testing OpenAI API connection...');
        const openai = new OpenAI({
            apiKey: process.env.OPENAI_API_KEY,
            fetchOptions: { dispatcher }
        });

        try {
            const models = await openai.models.list();
            console.log(`Success! OpenAI reachable. Found ${models.data.length} models.`);
        } catch (e) {
            console.error('OpenAI connection failed:', e.message);
            if (e.cause) console.error('Cause:', e.cause);
        }

    } catch (err) {
        console.error('Fatal error during test:', err);
    }
}

testProxy();
