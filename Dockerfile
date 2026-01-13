FROM node:20-bullseye

# Install FFmpeg and fonts
RUN apt-get update && apt-get install -y \
    ffmpeg \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

# Create temp directory
RUN mkdir -p temp

CMD ["npm", "start"]
