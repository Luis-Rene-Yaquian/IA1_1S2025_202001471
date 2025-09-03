% ======== rules.pl (NO lo sobreescribe el Admin) ========

% --- Directivas ---
:- dynamic(presente/2).
:- dynamic(alergia/1).
:- dynamic(cronica/1).
:- dynamic(enf_contra_medicamento/2).

% Hechos estáticos vienen del .pl de Admin:
%   sintoma(S).
%   enfermedad(Id, "Nombre", Sistema, Tipo).
%   enf_sintoma(Enf, Sintoma).
%   medicamento(Med).
%   trata(Med, Enf).
%   contraindicado(Med, Cond).
%   enf_contra_medicamento(Enf, Med).   % opcional

% -------------------------------------------------------------------
%            Severidad normalizada y utilidades básicas
% -------------------------------------------------------------------
peso(leve, 1).
peso(moderado, 2).
peso(severo, 3).

% presentepeso(S,P): normaliza presente(S, X) a número
presentepeso(S, P) :- presente(S, P), number(P), !.
presentepeso(S, P) :- presente(S, Sev), atom(Sev), peso(Sev, P).

peso_max_por_sintoma(3).

member(X, [X|_]).
member(X, [_|T]) :- member(X, T).

sum_pairs([], 0).
sum_pairs([(_,P)|T], S) :- sum_pairs(T, S1), S is S1 + P.

% -------------------------------------------------------------------
%                     Afinidad por enfermedad
% -------------------------------------------------------------------
% Lista única de síntomas requeridos por Enf
reqs_enf(Enf, Reqs) :-
    ( setof(S, enf_sintoma(Enf, S), S0) -> true ; S0 = [] ),
    sort(S0, Reqs).

max_puntaje_enf(Enf, Max) :-
    reqs_enf(Enf, Reqs),
    length(Reqs, N),
    peso_max_por_sintoma(PM),
    Max is N * PM.

% --- helpers para tomar el máximo de una lista ordenada ---
last_([X], X).
last_([_|T], X) :- last_(T, X).

% máximo peso observado para un síntoma presente (usando presentepeso/2)
max_peso_sintoma(S, P) :-
    setof(W, presentepeso(S, W), Ws),   % Ws queda ordenada asc
    last_(Ws, P).

% Puntaje real con los presentes normalizados (usa el peso máximo por síntoma)
% Matched: solo la lista de síntomas que contaron
puntaje_enf(Enf, Puntaje, Matched) :-
    reqs_enf(Enf, Reqs),
    findall((S,P),
        ( member(S, Reqs),
          max_peso_sintoma(S, P)        % solo entra si S está presente
        ),
        Pairs0),
    sort(Pairs0, Pairs),                % dedup por si repitiera (S,P)
    sum_pairs(Pairs, Puntaje),
    findall(S, member((S,_), Pairs), Matched).

% Afinidad en porcentaje (0..100)
afinidad(Enf, Afinidad, Matched) :-
    max_puntaje_enf(Enf, Max),
    ( Max =:= 0 -> Afinidad = 0, Matched = []
    ; puntaje_enf(Enf, Puntaje, Matched),
      Afinidad is round(Puntaje * 100 / Max)
    ).

% -------------------------------------------------------------------
%                            Urgencia
% -------------------------------------------------------------------
has_severe :- presentepeso(_, W), W >= 3.

symptom_count(N) :- findall(1, presentepeso(_, _), L), length(L, N).

% Banderas rojas absolutas
urgencia("Atención prioritaria") :-
    ( presentepeso(disnea, P), P >= 2
    ; presentepeso(dolor_pecho, _) ), !.

% Si hay cualquier síntoma severo (aunque no sea disnea/dolor_pecho)
urgencia("Consulta recomendada") :-
    has_severe, !.

% Si no hay severos pero hay 3+ síntomas presentes
urgencia("Consulta recomendada") :-
    symptom_count(N), N >= 3, !.

% Caso base
urgencia("Observación recomendada").

% -------------------------------------------------------------------
%                    Medicamento seguro / bloqueos
% -------------------------------------------------------------------
bloqueado_por_alergia(Med) :- alergia(Cond),  contraindicado(Med, Cond).
bloqueado_por_cronica(Med) :- cronica(Cond),  contraindicado(Med, Cond).
bloqueado_por_enf(Enf, Med) :- enf_contra_medicamento(Enf, Med).

% Trata y no está bloqueado
medicamento_seguro(Enf, Med) :-
    trata(Med, Enf),
    \+ bloqueado_por_alergia(Med),
    \+ bloqueado_por_cronica(Med),
    \+ bloqueado_por_enf(Enf, Med).

% -------------------------------------------------------------------
%                 (Opcional) Detalle para depurar
% -------------------------------------------------------------------
detalle_enf(Enf, Reqs, Matched, Puntaje, Max, Afinidad) :-
    reqs_enf(Enf, Reqs),
    puntaje_enf(Enf, Puntaje, Matched),
    max_puntaje_enf(Enf, Max),
    afinidad(Enf, Afinidad, _).
