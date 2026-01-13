const OpenAI = require('openai');
const fs = require('fs');
const { ProxyAgent } = require('undici');

const openaiConfig = {
  apiKey: process.env.OPENAI_API_KEY
};

if (process.env.PROXY) {
  const dispatcher = new ProxyAgent(process.env.PROXY);
  openaiConfig.fetchOptions = {
    dispatcher
  };
}

const openai = new OpenAI(openaiConfig);

/**
 * Транскрибирует аудио в текст с временными метками
 * @param {string} audioPath - Путь к аудиофайлу
 * @returns {Promise<Object>} Объект с текстом и сегментами с временными метками
 */
async function transcribeAudio(audioPath) {
  try {
    const transcription = await openai.audio.transcriptions.create({
      file: fs.createReadStream(audioPath),
      model: 'whisper-1',
      response_format: 'verbose_json',
      timestamp_granularities: ['word']
    });

    // Форматируем результат для использования в видео
    const result = {
      text: transcription.text,
      words: transcription.words || []
    };

    // Если нет данных по словам, создаем простой сегмент
    if (result.words.length === 0) {
      result.words = [{
        word: transcription.text,
        start: 0,
        end: 5 // примерная длительность
      }];
    }

    return result;
  } catch (error) {
    console.error('Ошибка транскрипции:', error);
    // Возвращаем пустой результат в случае ошибки
    return {
      text: '',
      words: []
    };
  }
}

module.exports = { transcribeAudio };
