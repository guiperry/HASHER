#ifndef SPACY_WRAPPER_H
#define SPACY_WRAPPER_H

#include <stdbool.h>
#include <stddef.h>

#ifdef __cplusplus
#include <string>

struct Token {
    std::string text;
    std::string lemma;
    std::string pos;
    std::string tag;
    std::string dep;
    bool is_stop;
    bool is_punct;
    int start;
    int end;
};

struct Entity {
    std::string text;
    std::string label;
    int start;
    int end;
};

extern "C" {
#endif

typedef struct {
    const char* text;
    const char* lemma;
    const char* pos;
    const char* tag;
    const char* dep;
    bool is_stop;
    bool is_punct;
    int start;
    int end;
} CToken;

typedef struct {
    CToken* tokens;
    size_t count;
} TokenArray;

typedef struct {
    const char* text;
    const char* label;
    int start;
    int end;
} CEntity;

typedef struct {
    CEntity* entities;
    size_t count;
} EntityArray;

typedef struct {
    char** sentences;
    size_t count;
} SentenceArray;

typedef struct {
    const char* text;
    const char* root_text;
    const char* root_dep;
    int start;
    int end;
} CChunk;

typedef struct {
    CChunk* chunks;
    size_t count;
} ChunkArray;

typedef struct {
    double* vector;
    size_t size;
    bool has_vector;
} CVectorData;

typedef struct {
    const char* morph_key;
    const char* morph_value;
} CMorphFeature;

typedef struct {
    CMorphFeature* features;
    size_t count;
} MorphArray;

int spacy_init(const char* model_name);
void spacy_cleanup();

TokenArray spacy_tokenize(const char* text);
void free_token_array(TokenArray* arr);

EntityArray spacy_extract_entities(const char* text);
void free_entity_array(EntityArray* arr);

SentenceArray spacy_split_sentences(const char* text);
void free_sentence_array(SentenceArray* arr);

ChunkArray spacy_get_noun_chunks(const char* text);
void free_chunk_array(ChunkArray* arr);

CVectorData spacy_get_vector(const char* text);
void free_vector_data(CVectorData* vec);

double spacy_similarity(const char* text1, const char* text2);

MorphArray spacy_get_morphology(const char* text);
void free_morph_array(MorphArray* arr);

#ifdef __cplusplus
}
#endif

#endif